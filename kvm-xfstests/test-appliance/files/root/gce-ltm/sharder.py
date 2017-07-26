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
from string import ascii_lowercase as alphabetlc
from cmdparser import LTMParser
import gce_funcs
from shard import Shard

# Note that during DST, most of these are +1'd.
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
    self.extra_opts = ' '.join(self.parser.extra_cmds)
    self.extra_cmds_b64 = base64.encodestring(self.extra_opts)

  def simple_sharding(self, max_cmds=0):
    """Splits all configs into max_cmds groups linearly.

    e.g. if there are 11 configs, the first group will have 4 configs,
    the second group will have 4, and the last will have 3.

    Args:
      max_cmds: The maximum number of individual commands to create.
                Each group will be a single command.

    Returns:
      A list of config strings.
    """
    all_configs = ['%s/%s' % (fs, cfg) for fs in self.parser.fsconfigs.keys()
                   for cfg in self.parser.fsconfigs[fs]]
    if max_cmds <= 0 or len(all_configs) <= max_cmds:
      return all_configs
    # split all_configs into runs of len/max_cmds
    # (+1 if needed to even out any remainders)
    st = 0
    ex = len(all_configs) % max_cmds
    jmp = len(all_configs) // max_cmds
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

  def get_shards(self, max_shards=0):
    """Splits up configs into shards and returns them.

    Args:
      max_shards: The maximum number of shards to create.

    Returns:
      A list of shards, ready to be started.
    """
    logging.debug('entered get_shards, max_shards is %d', max_shards)
    sharded_cfgs = self.simple_sharding(max_cmds=1)
    all_shards = []
    for i, test_config in enumerate(sharded_cfgs):
      shard_id = alphabetlc[i//26] + alphabetlc[i%26]
      shard = Shard(test_config, self.extra_cmds_b64, shard_id,
                    self.test_run_id, self.shard_log_dir_path,
                    gce_project=self.gce_proj_id)
      all_shards.append(shard)
    return all_shards

# end class Sharder
