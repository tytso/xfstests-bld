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
import base64
from datetime import datetime
import fcntl
import logging
from multiprocessing import Process
import os
import random
import shutil
from subprocess import call
from time import sleep
import gce_funcs
from ltm import LTM
from sharder import Sharder
from google.cloud import storage


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
    self.agg_results_dir = '%s/results-%s-%s/' % (
        self.log_dir_path, LTM.ltm_username, self.id)
    self.agg_results_filename = '%sresults.%s-%s' % (
        self.log_dir_path, LTM.ltm_username, self.id)
    self.kernel_version = 'unknown_kernel_version'

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
    """Moves all of the shard results into the aggregate results dir.

    For each shard, this function looks for the shard's advertised results
    directory. If it isn't present, then look for the shard's serial port
    dump. If neither are present, simply skip the shard and log a warning.

    In addition, this function also concatenates certain top-level results
    files from each shard, e.g. runtests.log. This wlil be output into a
    file at the top level (where one would normally find a runtests.log for
    a non-LTM run).
    """
    logging.info('Aggregating sharded results')
    LTM.create_log_dir(self.agg_results_dir)

    for shard in self.shards:
      logging.info('Moving %s into aggregate test results folder',
                   shard.unpacked_results_dir)
      shard.finished_with_serial = False
      if os.path.exists(shard.unpacked_results_dir):
        shutil.move(shard.unpacked_results_dir, self.agg_results_dir +
                    shard.id)
      elif os.path.exists(shard.unpacked_results_serial):
        shutil.move(shard.unpacked_results_serial, self.agg_results_dir +
                    shard.id + '.serial')
        shard.finished_with_serial = True
      else:
        logging.warning('Could not find %s or %s, shard may not have completed '
                        'correctly', shard.unpacked_results_dir,
                        shard.unpacked_results_serial)
        continue

    # concatenate files from subdirectories into a top-level
    # aggregate file at self.agg_results_dir + filename
    # Files to concat: runtests.log, cmdline, summary, failures, run-stats
    # testrunid

    for c in ['runtests.log', 'cmdline', 'summary', 'failures', 'run-stats',
              'testrunid', 'kernel_version']:
      self.__concatenate_shard_files(c)

    # read the first kernel_version we find (if we find any)
    for shard in self.shards:
      try:
        with open(self.agg_results_dir + '%s/%s'
                  % (shard.id, 'kernel_version'),
                  'r') as f:
          self.kernel_version = f.read().strip()
        break
      except IOError:
        continue
    return

  def __concatenate_shard_files(self, filename):
    """Concatenate all shard files of a given filename.

    This function takes in a filename argument, and looks through each shard's
    entry in the aggregate results directories for the filename.

    This function writes a new file of "filname" at the top level of the
    aggregate results directory, whose contents are those of each shard,
    concatenated.

    Args:
      filename: The name of the file to create, and the name to look for in
                each shard.
    """
    logging.debug('Concatenating shard file %s', filename)
    fa = open('%s%s' % (self.agg_results_dir, filename), 'w')

    fa.write('LTM aggregate file for %s\n' % filename)
    fa.write('Test run ID %s\n' % self.id)
    fa.write('Aggregate results from %d shards\n' % len(self.shards))

    for shard in self.shards:
      fa.write('\n============SHARD %s============\n' % shard.id)
      fa.write('============CONFIG: %s\n\n' % shard.test_fs_cfg)
      if shard.finished_with_serial:
        fa.write('Shard %s did not finish properly. '
                 'Serial data is present in the results dir.\n')
      else:
        try:
          with open(self.agg_results_dir + '%s/%s'
                    % (shard.id, filename),
                    'r') as f:
            fa.write(f.read())
        except IOError:
          logging.warning('Could not open/read file %s for shard %s',
                          filename, shard.id)
          fa.write('Could not open/read file %s for shard %s\n'
                   % (filename, shard.id))
      fa.write('\n==========END SHARD %s==========\n' % shard.id)
    fa.close()

  def __create_ltm_info(self):
    """Creates an ltm-info file and a ltm_logs directory in the results dir.

    This function creates an easily-readable info file at the top level of the
    results dir called "ltm-info", with information about each shard. This
    also aggregates the TestRun and Shard's logging (from the LTM) into the
    ltm_logs file.
    """
    # Additional file to create:
    # self.agg_results_dir/ltm-info
    # (original cmd, and what was run on each shard)
    logging.info('Start')
    fa = open(self.agg_results_dir + 'ltm-info', 'w')
    results_ltm_log_dir = self.agg_results_dir + 'ltm_logs/'
    LTM.create_log_dir(results_ltm_log_dir)

    fa.write('LTM test run ID %s\n' % self.id)
    fa.write('Original command: %s\n'
             % base64.decodestring(self.orig_cmd_b64))
    fa.write('Aggregate results from %d shards\n' % len(self.shards))
    fa.write('SHARD INFO:\n\n')
    for shard in self.shards:
      fa.write('SHARD %s\n' % shard.id)
      fa.write('instance name: %s\n' % shard.instance_name)
      fa.write('split config: %s\n' % shard.test_fs_cfg)
      fa.write('gce command executed: %s\n\n' % str(shard.gce_xfstests_cmd))

      shutil.copy2(shard.log_file_path, results_ltm_log_dir)
      shutil.copy2(shard.cmdlog_file_path, results_ltm_log_dir)
    shutil.copy2(self.log_file_path, results_ltm_log_dir)
    fa.close()

  def __pack_results_file(self):
    """tars and xz's the aggregate results directory, uploading to GS bucket.

    Calls tar and xz to pack the results directory the same way that a test
    appliance would. After uploading the results and summary file, the local
    copy of the directory will be deleted to preserve space.
    """
    logging.info('Start')
    logging.info('calling tar')
    call(['tar', '-C', self.agg_results_dir, '-cf',
          '%s.tar' % (self.agg_results_filename), '.'])

    logging.info('tarfile %s.tar', self.agg_results_filename)
    tarfile = open('%s.tar' % (self.agg_results_filename), 'r')

    logging.info('finalfile %s.tar.xz', self.agg_results_filename)
    finalfile = open('%s.tar.xz' % (self.agg_results_filename), 'w')

    logging.info('calling xz from tarfile to finalfile')
    call(['xz', '-6e'], stdin=tarfile, stdout=finalfile)

    finalfile.close()
    tarfile.close()
    storage_client = storage.Client()
    bucket_name = gce_funcs.get_gs_bucket().strip()
    bucket = storage_client.lookup_bucket(bucket_name)
    logging.info('Uploading repacked results .tar.xz file')

    with open('%s.tar.xz' % (self.agg_results_filename), 'r') as f:
      bucket.blob(self.__gce_results_filename(self.kernel_version)
                 ).upload_from_file(f)

    # Upload the summary file as well.
    with open(self.agg_results_dir + 'summary', 'r') as f:
      bucket.blob(self.__gce_summary_filename(self.kernel_version)
                 ).upload_from_file(f)

    logging.info('Deleting local .tar and .tar.xz files')
    os.remove('%s.tar' % self.agg_results_filename)
    os.remove('%s.tar.xz' % self.agg_results_filename)
    logging.info('Deleting local aggregate results directory')
    shutil.rmtree(self.agg_results_dir)

  def __gce_results_filename(self, kernel_version):
    return 'results.%s-%s.%s.tar.xz' % (
        LTM.ltm_username, self.id, kernel_version)

  def __gce_summary_filename(self, kernel_version):
    return 'summary.%s-%s.%s.txt' % (
        LTM.ltm_username, self.id, kernel_version)

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
