"""Shard class to monitor and collect results from a single test VM.

Shards are created by the sharder. Configurations, extra commands, and the
shard ID are all assigned by the sharder.
Calling run() on this object will spawn a subprocess to do the shard's work.

The subprocess starts with calling gce-xfstests with the specified config to
launch a child VM.
The shard process then waits for the sharded test run to
complete, by checking for the existence of the VM in GCE every 60 seconds.
When the test VM is no longer present, the shard will unpack the uploaded
results directory from GCS into the LTM's local filesystem before exiting the
process.
"""
import base64
import logging
from multiprocessing import Process
from subprocess import call
import gce_funcs
from ltm import LTM


class Shard(object):
  """Shard class."""

  def __init__(self, test_fs_cfg, extra_cmds_b64, shard_id, test_run_id,
               log_dir_path, gce_zone=None, gs_bucket=None, gce_project=None):
    if (not isinstance(extra_cmds_b64, basestring) or
        not isinstance(shard_id, basestring) or
        not isinstance(test_run_id, basestring)):
      raise TypeError
    logging.debug('Creating Shard instance %s', shard_id)

    self.extra_cmds_b64 = extra_cmds_b64
    self.test_run_id = test_run_id
    self.id = shard_id
    self.test_fs_cfg = test_fs_cfg
    self.config_cmd_arr = ['-c', test_fs_cfg]
    self.extra_cmd_arr = base64.decodestring(extra_cmds_b64).strip().split(' ')
    if gce_zone:
      self.gce_zone = gce_zone
    else:
      self.gce_zone = gce_funcs.get_gce_zone().strip()
    if gs_bucket:
      self.gs_bucket_name = gs_bucket
    else:
      self.gs_bucket_name = gce_funcs.get_gs_bucket().strip()
    if gce_project:
      self.gce_project = gce_project
    else:
      self.gce_project = gce_funcs.get_proj_id().strip()
    self.instance_name = 'xfstests-%s-%s-%s' % (
        LTM.ltm_username, self.test_run_id, self.id)

    gce_xfstests_cmd = ['gce-xfstests', '--instance-name', self.instance_name]
    if gce_zone:
      gce_xfstests_cmd.extend(['--gce-zone', gce_zone])
    if gs_bucket:
      gce_xfstests_cmd.extend(['--gs-bucket', gs_bucket])
    gce_xfstests_cmd.extend(['--image-project', self.gce_project])
    gce_xfstests_cmd.extend(self.config_cmd_arr)
    gce_xfstests_cmd.extend(self.extra_cmd_arr)
    self.gce_xfstests_cmd = gce_xfstests_cmd

    # LOG/RESULTS VARIABLES
    self.log_file_path = log_dir_path + self.id
    self.cmdlog_file_path = self.log_file_path + '.cmdlog'

    logging.debug('Created Shard instance %s', shard_id)
  # end __init__

  def run(self):
    logging.info('Spawning child process for shard %s', self.id)
    self.process = Process(target=self.__run)
    self.process.start()
    return

  def __run(self):
    """Main function for a shard.

    This function will be called in a separate running process, after
    run is called. The function makes an explicit call to exit() after
    finishing the procedure to exit the process.
    This function should not be called directly.
    """
    logging.info('Child process spawned for shard %s', self.id)
    logging.info('Switch logging to shard file')
    logging.getLogger().handlers = []  # clear handlers for new process
    logging.basicConfig(
        filename=self.log_file_path,
        format='[%(levelname)s:%(asctime)s %(filename)s:%(lineno)s-'
               '%(funcName)s()] %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S', level=logging.INFO)
    self.__start()
    self.__monitor()
    self.__finish()
    logging.info('Exiting monitor process for shard %s', self.id)
    exit()

  def __start(self):
    logging.info('Starting Shard %s at %s', self.id, self.test_run_id)
    logging.debug('opening log file %s', self.cmdlog_file_path)
    f = open(self.cmdlog_file_path, 'w')
    logging.info('Calling command %s', str(self.gce_xfstests_cmd))
    returned = call(self.gce_xfstests_cmd, stdout=f, stderr=f)
    f.close()
    logging.info('Command returned %s', returned)
    return

  def __monitor(self):
    logging.info('Entered monitor.')
    return

  def __finish(self):
    logging.info('Getting results file from gcs bucket')
    return

### end class Shard

