"""The TestRunManager sets up and manages a single test run.

The only arguments to it are the originally executed command line.
This original command can contain the "ltm" flag itself as well as other
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
import shutil
from subprocess import call
import sys
from time import sleep
from urllib2 import HTTPError

import gce_funcs
from ltm import LTM
import sendgrid
from sharder import Sharder
from google.cloud import storage
from gen_results_summary import gen_results_summary

class TestRunManager(object):
  """TestRunManager class.

  The TestRunManager on construction will acqurie a unique testrunid, create
  a Sharder and get shards. After this, when the run() function is called, the
  testrunmanager will spawn a child process in which it will run the test run,
  monitor its shards, and aggregate the results.
  """

  def __init__(self, orig_cmd, opts=None):
    logging.info('Building new Test Run')
    logging.info('Getting unique test run id..')
    test_run_id = get_unique_test_run_id()
    logging.info('Creating new TestRun with id %s', test_run_id)

    self.id = test_run_id
    self.orig_cmd = orig_cmd.strip()
    self.log_dir_path = LTM.test_log_dir + '%s/' % test_run_id
    self.log_file_path = self.log_dir_path + 'run.log'
    self.agg_results_dir = '%sresults-%s-%s/' % (
        self.log_dir_path, LTM.ltm_username, self.id)
    self.agg_results_filename = '%sresults.%s-%s' % (
        self.log_dir_path, LTM.ltm_username, self.id)
    self.kernel_version = 'unknown_kernel_version'

    LTM.create_log_dir(self.log_dir_path)
    logging.info('Created new TestRun with id %s', self.id)
    self.shards = []

    region_shard = True
    self.gs_bucket = gce_funcs.get_gs_bucket().strip()
    self.bucket_subdir = gce_funcs.get_bucket_subdir().strip()
    self.gs_kernel = None
    self.report_receiver = None
    if opts and 'no_region_shard' in opts:
      region_shard = False
    if opts and 'bucket_subdir' in opts:
      self.bucket_subdir = opts['bucket_subdir'].strip()
    if opts and 'gs_kernel' in opts:
      self.gs_kernel = opts['gs_kernel'].strip()
    if opts and 'report_email' in opts:
      self.report_receiver = opts['report_email'].strip()
    # Other shard opts could be passed here.

    self.sharder = Sharder(self.orig_cmd, self.id, self.log_dir_path,
                           self.gs_bucket, self.bucket_subdir, self.gs_kernel)
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

  def _setup_logging(self):
    logging.info('Move logging to testrun file %s', self.log_file_path)
    logging.getLogger().handlers = []  # clear log handlers
    logging.basicConfig(
        filename=self.log_file_path,
        format='[%(levelname)s:%(asctime)s %(filename)s:%(lineno)s-'
               '%(funcName)s()] %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S', level=logging.DEBUG)
    sys.stderr = sys.stdout = open(self.log_file_path, 'a')

  def __run(self):
    """Main method for a testrun.
    """
    logging.info('Child process spawned for testrun %s', self.id)
    self._setup_logging()
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
    """Completion method of the testrunmanager, run after shards complete.

    This method will attempt to aggregate the results of every shard by moving
    either the shard's results directory or its serial dump into an aggregate
    folder. If none of the shards created any results or serial dumps, it will
    exit early.

    After aggregating the results, this will write an additional info file
    about the testrun, re-tar the aggregate directory, and re-upload this
    tarball to the GS bucket (and potential subdir)
    """
    logging.info('Entered finish()')

    any_results = self.__aggregate_results()
    if any_results:
      self.__create_ltm_info()
      self.__create_ltm_run_stats()
      gen_results_summary(self.agg_results_dir,
                          os.path.join(self.agg_results_dir, 'report'))
      self.__email_report()
      self.__pack_results_file()
    else:
      logging.error('Finishing without uploading anything.')

    self.__cleanup()
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

    Returns:
      A boolean. If the boolean is false, none of the shards correctly
      created results or a serial dump. Otherwise at least one of the
      shards completed meaningfully.
    """
    logging.info('Aggregating sharded results')
    LTM.create_log_dir(self.agg_results_dir)

    no_results_available = True
    for shard in self.shards:
      logging.info('Moving %s into aggregate test results folder',
                   shard.unpacked_results_dir)
      found_outputs = False
      shard.finished_with_serial = False
      if os.path.exists(shard.unpacked_results_dir):
        shutil.move(shard.unpacked_results_dir, self.agg_results_dir +
                    shard.id)
        no_results_available = False
        found_outputs = True
      if os.path.exists(shard.serial_output_file_path):
        shutil.move(shard.serial_output_file_path, self.agg_results_dir +
                    shard.id + '.serial')
        shard.finished_with_serial = True
        no_results_available = False
        found_outputs = True
      if not found_outputs:
        logging.warning('Could not find results for shard at %s or %s, shard '
                        'may not have completed correctly',
                        shard.unpacked_results_dir,
                        shard.serial_output_file_path)
        continue
    if no_results_available:
      logging.error('No results are available for any of the shards.')
      logging.error('All shard processes exited without creating any results '
                    'or serial dumps.')
      return False
    # concatenate files from subdirectories into a top-level
    # aggregate file at self.agg_results_dir + filename

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
    return True

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
      try:
        with open(self.agg_results_dir + '%s/%s'
                  % (shard.id, filename),
                  'r') as f:
          fa.write(f.read())
      except IOError:
        if shard.finished_with_serial:
          fa.write('Shard %s did not finish properly. '
                   'Serial data is present but not results.\n')
        else:
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
    logging.info('Entered create_ltm_info')
    fa = open(self.agg_results_dir + 'ltm-info', 'w')
    results_ltm_log_dir = self.agg_results_dir + 'ltm_logs/'
    LTM.create_log_dir(results_ltm_log_dir)

    fa.write('LTM test run ID %s\n' % self.id)
    fa.write('Original command: %s\n' % self.orig_cmd)
    fa.write('Aggregate results from %d shards\n' % len(self.shards))
    fa.write('SHARD INFO:\n\n')
    for shard in self.shards:
      fa.write('SHARD %s\n' % shard.id)
      fa.write('instance name: %s\n' % shard.instance_name)
      fa.write('split config: %s\n' % shard.test_fs_cfg)
      fa.write('gce command executed: %s\n\n' % str(shard.gce_xfstests_cmd))

      try:
        shutil.move(shard.log_file_path, results_ltm_log_dir)
      except IOError:
        logging.warning('Could not move log for shard %s', shard.id)
        logging.warning('log file path was %s', shard.log_file_path)
      try:
        shutil.move(shard.cmdlog_file_path, results_ltm_log_dir)
      except IOError:
        logging.warning('Could not move cmdlog for shard %s', shard.id)
        logging.warning('cmdlog file path was %s', shard.cmdlog_file_path)
    fa.close()
    # All logging after this point will be written to the logfile in
    # results_ltm_log_dir
    shutil.move(self.log_file_path, results_ltm_log_dir)
    self._setup_logging()  # move the logs back to the right place
    # This is necessary so that diagnostics can be done in the event that
    # the LTM server fails after having created the LTM info file.
    # Otherwise the root logger's open file descriptors will still point
    # to the file after it has been moved, which gets removed in the
    # __cleanup function.
    logging.info('Finished creating ltm-info')

  def __create_ltm_run_stats(self):
    """Creates an ltm-run-stats file in the results dir.

    This function creates a easily machine-readable run-stats file at
    the top level of the results dir called "ltm-run-stats", with
    information about the overall LTM test run.

    """
    logging.info('Entered create_ltm_run_stats')
    fa = open(os.path.join(self.agg_results_dir, 'ltm-run-stats'), 'w')

    fa.write('TESTRUNID: %s-%s\n' % (LTM.ltm_username, self.id))
    fa.write('CMDLINE: %s\n' % self.orig_cmd)
    fa.close()
    logging.info('Finished creating ltm-run-stats')

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
    bucket = storage_client.lookup_bucket(self.gs_bucket)
    logging.info('Uploading repacked results .tar.xz file')

    with open('%s.tar.xz' % (self.agg_results_filename), 'r') as f:
      bucket.blob(self.__gce_results_filename(self.kernel_version)
                 ).upload_from_file(f)

    # Upload the summary file as well.
    if gce_funcs.get_upload_summary():
      with open(self.agg_results_dir + 'summary', 'r') as f:
        bucket.blob(self.__gce_results_filename(self.kernel_version,
                                                summary=True)
                   ).upload_from_file(f)

  def __gce_results_filename(self, kernel_version, summary=False):
    bucket_subdir = 'results'
    if self.bucket_subdir:
      bucket_subdir = self.bucket_subdir
    if summary:
      return '%s/summary.%s-%s.%s.txt' % (
          bucket_subdir, LTM.ltm_username, self.id, kernel_version)
    return '%s/results.%s-%s.%s.tar.xz' % (
        bucket_subdir, LTM.ltm_username, self.id, kernel_version)

  def __cleanup(self):
    """Cleanup to be done after all shards are finished.

    This function cleans up the GS bucket by deleting the kernel image if it
    was specified to be a onerun. This is akin to the regular gce-xfstests
    test appliance deleting a "bzImage-*-onetime" image, except for the LTM
    exclusively.

    Other cleanup to be done after all shards are exited can be done here too
    """
    logging.info('Entered cleanup')
    logging.info('Deleting local .tar and .tar.xz files, if they exist')
    if os.path.isfile('%s.tar' % self.agg_results_filename):
      os.remove('%s.tar' % self.agg_results_filename)
      logging.debug('Deleted %s.tar', self.agg_results_filename)
    if os.path.isfile('%s.tar.xz' % self.agg_results_filename):
      os.remove('%s.tar.xz' % self.agg_results_filename)
      logging.debug('Deleted %s.tar.xz', self.agg_results_filename)
    logging.info('Deleting local aggregate results directory')
    shutil.rmtree(self.agg_results_dir)
    # gs_kernel looks like
    # gs://$GS_BUCKET/<optional subdir/>bzImage-<blah>-onerun
    if self.gs_kernel and self.gs_kernel.endswith('-onerun'):
      logging.info('deleting onerun kernel image %s', self.gs_kernel)
      blob_name = self.gs_kernel.split(self.gs_bucket)[1][1:]
      logging.info('blob name is %s', blob_name)
      storage_client = storage.Client()
      bucket = storage_client.lookup_bucket(self.gs_bucket)
      bucket.blob(blob_name).delete()
      logging.info('deleted blob %s, full path %s', blob_name, self.gs_kernel)
    logging.info('finished cleanup')
    return

  def __email_report(self):
    """Emails the testrun report to the report receiver.

    If no report receiver email is specified, or if the api key is not found,
    this function will just return.
    """
    logging.info('Entered email report')
    if not self.report_receiver:
      logging.info('No destination for report to be sent to')
      return
    sendgrid_api_key = gce_funcs.get_sendgrid_api_key()
    if not sendgrid_api_key:
      logging.warning('No sendgrid api key found. Can\'t send email.')
      return
    logging.debug('Got sendgrid api key')
    email_subject = 'xfstests results %s-%s %s' % (
        LTM.ltm_username, self.id, self.kernel_version)
    report_sender = (gce_funcs.get_email_report_sender()
                     or self.report_receiver)
    source_email = sendgrid.helpers.mail.Email(report_sender)
    dest_email = sendgrid.helpers.mail.Email(self.report_receiver)
    logging.debug('email_subject %s, report_sender %s, report_receiver %s',
                  email_subject, report_sender, self.report_receiver)
    try:
      logging.info('Reading reports file as e-mail contents')
      with open(os.path.join(self.agg_results_dir, 'report'), 'r') as ff:
        report_content = sendgrid.helpers.mail.Content('text/plain', ff.read())
    except IOError:
      logging.warning('Unable to read report file for report contents')
      report_content = sendgrid.helpers.mail.Content(
          'text/plain',
          'Could not read contents of report file')
    sg = sendgrid.SendGridAPIClient(apikey=sendgrid_api_key)
    full_email = sendgrid.helpers.mail.Mail(source_email, email_subject,
                                            dest_email, report_content)

    try:
      response = sg.client.mail.send.post(request_body=full_email.get())
    except HTTPError as err:
      # Probably a bad API key. Possible other causes could include sendgrid's
      # API being unavailable, or any other errors that come from the
      # api.sendgrid.com/mail/send endpoint.
      # Docs:
      # https://sendgrid.com/docs/API_Reference/Web_API_v3/Mail/index.html
      # https://sendgrid.com/docs/API_Reference/Web_API_v3/Mail/errors.html
      # Common probable occurrences:
      # 401 unauthorized, 403 forbidden, 413 payload too large, 429 too many
      # requests, 500 server unavailable, 503 v3 api unavailable.
      # also, 200 OK and 202 ACCEPTED
      response = err

    if response.status_code/100 != 2:
      logging.error('Mail send failed, http error code %s',
                    str(err.status_code))
      logging.error('Headers:')
      logging.error(str(err.headers))
      logging.error('Body:')
      logging.error(str(err.body))
      logging.error('Reason:')
      logging.error(str(err.reason))
      return

    logging.info('Sent report to %s', self.report_receiver)
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
