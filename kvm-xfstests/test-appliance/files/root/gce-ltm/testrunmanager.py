"""The TestRunManager sets up and manages a single test run.

The only arguments to it are the originally executed command line in base64,
and options.
This original command will contain the "--ltm" flag itself as well as other
options that aren't used on the LTM (the command parser will remove those for
running on the ltm).

On construction:
  assign unique test run ID from timestamp
  create log dir
  use sharder to create shard objects

On run:
  launch shard processes
  wait for shard processes to complete
  aggregate results files

The main usage of the class is:
x = TestRunManager(cmd)
x.run()

On construction, if any misconfigurations are discovered (e.g. a lack of
available quota for VMs in the GCE project) or if no
tests are to be run based on the passed in configs, errors may be thrown.

After construction, get_info can be used to obtain testrun information about
what shards will be spawned, and what the name of the test run is going to be.

Under normal circumstances, run() will spawn a subprocess, which when
exited should have uploaded an aggregated results file to the GCS bucket,
and should have cleaned up all of the shard's results and summary files.
"""
from datetime import datetime
import fcntl
import logging
from multiprocessing import Process
import os
import random
from time import sleep
from ltm import LTM
from sharder import Sharder


class TestRunManager(object):
  """TestRunManager class.

  The TestRunManager on construction will acqurie a unique testrunid, create
  a Sharder and get shards. After this, when the run() function is called, the
  testrunmanager will spawn a child process in which it will run the test run,
  monitor its shards, and aggregate the results.
  """

  def __init__(self, cmd_in_base64, opts=None):
    logging.info('Building new Test Run')
    logging.info('Getting unique test run id..')
    test_run_id = get_unique_test_run_id()
    logging.info('Creating new TestRun with id %s', test_run_id)

    self.id = test_run_id
    self.orig_cmd_b64 = cmd_in_base64
    self.log_dir_path = LTM.test_log_dir + '%s/' % test_run_id
    self.log_file_path = self.log_dir_path + 'run.log'

    LTM.create_log_dir(self.log_dir_path)
    logging.info('Created new TestRun with id %s', self.id)
    self.shards = []

    self.sharder = Sharder(self.orig_cmd_b64, self.id, self.log_dir_path)
    region_shard = True
    if opts and 'no_region_shard' in opts:
      region_shard = False
    # Other shard opts could be passed here.
    self.shards = self.sharder.get_shards(region_shard=region_shard)

  def run(self):
    logging.info('Entered run()')
    logging.info('Spawning child process for testrun %s', self.id)
    self.process = Process(target=self.__run)
    self.process.start()
    return

  def get_info(self):
    """Get info about how the testrun is to be run.

    Info includes the testrunid, number of shards, and information about each
    individual shard.

    Returns:
      info: a dictionary.
    """
    info = {}
    info['num_shards'] = len(self.shards)
    info['shard_info'] = []
    info['id'] = self.id
    for i, shard in enumerate(self.shards):
      info['shard_info'].append(
          {'index': i,
           'shard_id': shard.id,
           'cfg': shard.test_fs_cfg,
           'zone': shard.gce_zone})
    return info

  def __run(self):
    """Main method for a testrun.
    """
    logging.info('Child process spawned for testrun %s', self.id)
    logging.info('Switch logging to testrun file %s', self.log_file_path)
    logging.getLogger().handlers = []  # clear log handlers for new process
    logging.basicConfig(
        filename=self.log_file_path,
        format='[%(levelname)s:%(asctime)s %(filename)s:%(lineno)s-'
               '%(funcName)s()] %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S', level=logging.DEBUG)

    self.__start()
    self.__wait_for_shards()
    self.__finish()
    logging.info('Exiting process for testrun %s', self.id)
    exit()

  def __start(self):
    """Launches all of the shards.

    This function will simply launch all of the shards that this testrunmanager
    currently holds. A 0.5 second sleep is done between launching shards to
    avoid hitting the GCE API too hard (each shard runs its own gce-xfstests
    command, which calls gcloud).
    """
    logging.info('Entered start()')
    logging.info('Spawning %d shards', len(self.shards))
    for shard in self.shards:
      shard.run()
      if not shard.process:
        logging.warning('Did not spawn shard %s', shard.id)
        logging.warning('Command was %s', str(shard.gce_xfstests_cmd))
      else:
        logging.info('Spawned shard %s', shard.id)
      # Throttle a bit between spawning shard processes. This is to avoid all of
      # the shards simultaneously executing gce-xfstests (gcloud) commands.
      # GCE might not like it if we try to spawn that many instances
      # simultaneously. Have run into "Backend Error" before, causing test to
      # not run (didn't spawn VM)
      sleep(0.5)
    return

  def __wait_for_shards(self):
    logging.info('Entered wait_for_shards()')
    for shard in self.shards:
      logging.info('Waiting for shard %s', shard.id)
      shard.process.join()
    return

  def __finish(self):
    logging.info('Entered finish()')

    self.__aggregate_results()
    self.__create_ltm_info()
    self.__pack_results_file()

    logging.info('finished.')
    return

  def __aggregate_results(self):
    logging.info('Aggregating sharded results')
    return

  def __create_ltm_info(self):
    return

  def __pack_results_file(self):
    return

### end class TestRunManager


def get_unique_test_run_id():
  """Grabs a unique test run ID from the current datetime.

  To ensure that two simultaneous test run requests are satisfied
  with DIFFERENT test run id values, we issue an flock on a temp file
  containing the last allocated ID. If the flock fails, we back off for
  rand(1,2) and try again.
  If the flock succeeds, we check if the last allocated ID is the same as
  datetime.now(). If it is, we hold onto the lock until datetime.now()
  returns an ID that is different from the one in the file, write it into
  the file, unlock the file, and return the ID.

  Returns:
    test_run_id: A test run ID (datetime as YYYYMMDDHHmmss), which will be
                 unique amongst all testrunIDs issued by this function.
  """
  with open('/tmp/ltm_id_lock', 'a+') as lockfile:
    while True:
      try:
        # Flock will block until the lock is available.
        fcntl.flock(lockfile, fcntl.LOCK_EX | fcntl.LOCK_NB)
        break
      except IOError:
        sleep(random.uniform(1.0, 2.0))
    previous_test_time = lockfile.read().strip()
    # If the test run ID we're about to issue is the same as the previous one
    # issued, we have to spin until the datetime call returns a different
    # string. We could consider calling a sleep(0.1) here as well, but spinning
    # will allow us to release the lock as soon as possible.
    test_run_id = get_datetime_test_run_id()
    while test_run_id == previous_test_time:
      test_run_id = get_datetime_test_run_id()
    # We now have a unique test_run_id. Write it into the file.
    lockfile.seek(0)
    lockfile.truncate()
    lockfile.write(test_run_id)
    # Need to force writing immediately, as there might be other
    # testrunmanagers with the file open but blocking on the flock.
    # flush and fsync will force a file update, which we do before
    # unlocking the file.
    lockfile.flush()
    os.fsync(lockfile)
    fcntl.flock(lockfile, fcntl.LOCK_UN)
  return test_run_id


def get_datetime_test_run_id():
  curtime = datetime.now()
  test_run_id = '%.4d%.2d%.2d%.2d%.2d%.2d' % (curtime.year, curtime.month,
                                              curtime.day, curtime.hour,
                                              curtime.minute,
                                              curtime.second)
  return test_run_id
