"""Sharding module to shard a gce-xfstests test run.

The Sharder takes a command in base64, invokes the command parser to understand
the command, and performs querying of GCE quota limitations (as well as
user-configurable limitations) to shard the configurations of the test run.

When get_shards is called, the work is done to query quotas and user-configured
limits. get_shards will then return a list of Shard objects, which will
be sorted in shard_id order ('aa','ab',etc...)

Currently unimplemented is the splitting of shards into multiple GCE
regions to utilize more of the available quota, and potentially utilize
preemtible VMs in regions with lower utilization. Shards will need to also be
made aware of this.

Additionally, if the cmdparser is updated to parse test sets (e.g. -g auto),
this info can be used to perform sharding.

If the sharder cannot create any shards due to limits, a ValueError will be
raised from get_shards.

"""
import base64
import logging
import random
from string import ascii_lowercase as alphabetlc
from cmdparser import LTMParser
import gce_funcs
import googleapiclient.discovery
from googleapiclient.errors import HttpError
from shard import Shard

# Note that during DST, most of these are +1'd.
# This is unused. In the future this might come in handy for deciding
# the best zone to run the tests in
timezones = {
    'us-central1': -6,  # Council Bluffs, Iowa, USA
    'us-west1': -8,  # The Dalles, Oregon, USA
    'us-east4': -5,  # Ashburn, Virginia, USA
    'us-east1': -5,  # Moncks Corner, South Carolina, USA
    'europe-west1': 1,  # St. Ghislain, Belgium
    'europe-west2': 0,  # London, U.K.
    'asia-southeast1': 8,  # Jurong West, Singapore
    'asia-east1': 8,  # Changhua County, Taiwan
    'asia-northeast1': 9,  # Tokyo, Japan
    'australia-southeast1': 10,  # Sydney, Australia
}


class Sharder(object):
  """Sharder class to query GCE quotas and create shards."""

  def __init__(self, cmd_b64, test_run_id, shard_log_dir_path):
    self.gce_proj_id = gce_funcs.get_proj_id()
    self.gce_zone = gce_funcs.get_gce_zone()
    self.gce_region = self.gce_zone[:-2]
    self.test_run_id = test_run_id
    self.shard_log_dir_path = shard_log_dir_path
    self.orig_cmd_b64 = cmd_b64
    self.parser = LTMParser(cmd_b64)
    self.compute = googleapiclient.discovery.build('compute', 'v1')
    self.extra_opts = ' '.join(self.parser.extra_cmds)
    self.extra_cmds_b64 = base64.encodestring(self.extra_opts)

  def __group_all_configs(self, max_groups=3):
    """Splits all configs into max_groups groups linearly.

    e.g. if there are 11 configs, the first group will have 4 configs,
    the second group will have 4, and the last will have 3.

    Args:
      max_groups: The maximum number of config groups to create.
                  Each group will be a single command.

    Returns:
      A list of config strings.
    """
    all_configs = ['%s/%s' % (fs, cfg) for fs in self.parser.fsconfigs.keys()
                   for cfg in self.parser.fsconfigs[fs] if 'dax' not in cfg]
    if max_groups <= 0 or len(all_configs) <= max_groups:
      return all_configs
    # split all_configs into runs of len/max_groups
    # (+1 if needed to even out any remainders)
    st = 0
    ex = len(all_configs) % max_groups
    jmp = len(all_configs) // max_groups
    configs = []
    while st < len(all_configs):
      if ex > 0:
        ex -= 1
        configs.append(','.join(all_configs[st:st+jmp+1]))
        st += jmp+1
      else:
        configs.append(','.join(all_configs[st:st+jmp]))
        st += jmp
    return configs

  def region_sharding(self):
    """Shards configs into any region with available quota.

    Attempt to split each config in all_configs into a separate VM, and run
    them all in different regions.

    We can assume that every config will consume 2 CPUs.
    By default, each region has 24 CPUs as a quota, and 500GB of pd-ssd.
    Only "size=large" configs use 56GB of pd-ssd, otherwise they usually use
    11-16GB.

    The CPU limit is usually the one which will be hit, or the external IP
    address limit.

    Returns:
      all_shards: A list of Shards, configured for any available region,
                  ready to be started
    Raises:
      ValueError: If the GCE project is out of quota and can't run any more
                  shards.
    """
    quotas = self.__get_all_region_quotas()
    total_max_shards = 0
    zones_to_use = []
    my_continent = self.gce_region.split('-')[0]
    preferred_zones = [i for i in quotas.iterkeys()
                       if i.startswith(my_continent)]
    other_zones = [i for i in quotas.iterkeys()
                   if not i.startswith(my_continent)]
    for k in preferred_zones:
      v = quotas[k]
      max_shards_for_zone = min(v[1:])
      total_max_shards += max_shards_for_zone
      zones_to_use.extend([v[0]]*max_shards_for_zone)
    random.shuffle(zones_to_use)
    other_zones_to_use = []
    for k in other_zones:
      v = quotas[k]
      max_shards_for_zone = min(v[1:])
      total_max_shards += max_shards_for_zone
      other_zones_to_use.extend([v[0]]*max_shards_for_zone)
    random.shuffle(other_zones_to_use)
    zones_to_use.extend(other_zones_to_use)
    if total_max_shards == 0:
      raise ValueError('GCE project is out of quota.')
    grouped_cfgs = self.__group_all_configs(max_groups=total_max_shards)
    all_shards = []
    for i, test_config in enumerate(grouped_cfgs):
      shard_id = alphabetlc[i//26] + alphabetlc[i%26]
      shard = Shard(test_config, self.extra_cmds_b64, shard_id,
                    self.test_run_id, self.shard_log_dir_path,
                    gce_zone=zones_to_use[i], gce_project=self.gce_proj_id)
      all_shards.append(shard)
    return all_shards

  def local_sharding(self, max_shards=0):
    """Shards into the same zone that the LTM is running in.

    Attempts to split each config into a separate VM, constraining shards to
    the current zone of the LTM. If max_shards is specified, this will
    create at most that many shards.

    Args:
      max_shards: Upper bound on the number of shards to create.

    Returns:
      all_shards: A list of shards for the current region, ready to be started.

    Raises:
      ValueError: If the region of the LTM is out of quota and can't run any
                  more shards.
    """
    logging.info('Sharding into own region %s', self.gce_region)
    all_shards = []
    [_, cpu_limit_shards, ip_limit_shards] = self.__get_region_info(
        self.gce_region)

    if max_shards <= 0:
      max_shards = cpu_limit_shards

    max_shards = min([max_shards, cpu_limit_shards, ip_limit_shards])
    logging.info('Max shards set to %d', max_shards)
    if max_shards == 0:
      # we have no quota left... we have to error out at this point.
      raise ValueError('GCE region %s is out of quota.' % self.gce_region)
    grouped_cfgs = self.__group_all_configs(max_groups=max_shards)

    for i, test_config in enumerate(grouped_cfgs):
      shard_id = alphabetlc[i//26] + alphabetlc[i%26]

      shard = Shard(test_config, self.extra_cmds_b64, shard_id,
                    self.test_run_id, self.shard_log_dir_path,
                    gce_project=self.gce_proj_id)

      all_shards.append(shard)
    return all_shards

  # region_shard=True will override max_shards
  def get_shards(self, region_shard=False, max_shards=0):
    """Splits up configs into shards and returns them.

    If region_shard is set to True, the sharder will consider all available
    regions with remaining quota. Otherwise, shards will only be in the zone
    (and region) that the LTM is running in.

    Args:
      region_shard: If set to True, will shard into any available region in
                    the project.
      max_shards: The maximum number of shards to create.

    Returns:
      A list of shards, ready to be started.
    """
    logging.debug('entered get_shards, max_shards is %d', max_shards)

    if region_shard:
      return self.region_sharding()
    else:
      return self.local_sharding(max_shards=max_shards)

  def __get_region_info(self, region, region_info=None):
    """Gets quota information for the given region.

    Args:
      region: The region to check quotas for.
      region_info: an optional parameter containing the region info returned
                   from the GCE python API. If this is None, the GCE python API
                   will be queried.

    Returns:
      zone: A randomly selected available zone in the region
      cpu_limit_shards: the number of shards that can be split into this region
                        considering the available CPU quota.
      ip_limit_shards: same as above, except for IP address quota.

    Raises:
      ValueError: If the given GCE region has no available zones.
    """
    logging.debug('get region quotas for region %s', region)
    if not region_info:
      try:
        region_info = self.compute.regions().get(project=self.gce_proj_id,
                                                 region=region).execute()
      except HttpError:
        logging.info('could not find region %s in project %s', region,
                     self.gce_proj_id)
        return
    zone_names = [x.split('/')[-1] for x in region_info['zones']]
    zone = None
    for z in zone_names:
      zone_info = self.compute.zones().get(project=self.gce_proj_id,
                                           zone=z).execute()
      if zone_info['status'] == 'UP':
        zone = zone_info['name']
    if not zone:
      raise ValueError('GCE region %s has no available zones' % region)
    for q in region_info['quotas']:
      if q['metric'] == 'CPUS':
        available_cpus = int(q['limit'] - q['usage'])
      if q['metric'] == 'IN_USE_ADDRESSES':
        available_ips = int(q['limit'] - q['usage'])

    cpu_limit_shards = available_cpus//2
    ip_limit_shards = available_ips
    logging.debug('region %s, cpu_limit %d, ip_limit %d',
                  region, cpu_limit_shards, ip_limit_shards)
    return [zone, cpu_limit_shards, ip_limit_shards]

  def __get_all_region_quotas(self):
    """Gets quotas for every region in the GCE project.

    Utilizes the GCE python APIs to get quota information for every region
    in the project.

    Returns:
      quotas: dict, maps 'region name' to a return value of
              self.__get_region_info
    """
    all_regions = self.compute.regions().list(
        project=self.gce_proj_id).execute()['items']
    quotas = {}
    for x in all_regions:
      if x['status'] == 'UP':
        try:
          quotas[x['name']] = self.__get_region_info(x['name'], x)
        except ValueError:
          quotas.pop(x['name'], None)
    return quotas

# end class Sharder
