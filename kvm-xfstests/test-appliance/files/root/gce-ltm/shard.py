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
import io
import logging
from multiprocessing import Process
import os
import shutil
from subprocess import call
from time import sleep
import gce_funcs
import googleapiclient.discovery
import googleapiclient.errors
from ltm import LTM
from google.cloud import storage


class Shard(object):
  """Shard class."""

  def __init__(self, test_fs_cfg, extra_cmds_b64, shard_id, test_run_id,
               log_dir_path, gce_zone=None, gs_bucket=None, gce_project=None,
               bucket_subdir=None):
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
      self.gs_bucket = gs_bucket
    else:
      self.gs_bucket = gce_funcs.get_gs_bucket().strip()
    if bucket_subdir:
      self.bucket_subdir = bucket_subdir
    else:
      self.bucket_subdir = gce_funcs.get_bucket_subdir().strip()
    if gce_project:
      self.gce_project = gce_project
    else:
      self.gce_project = gce_funcs.get_proj_id().strip()
    self.keep_dead_vm = gce_funcs.get_keep_dead_vm()
    self.instance_name = 'xfstests-%s-%s-%s' % (
        LTM.ltm_username, self.test_run_id, self.id)

    gce_xfstests_cmd = ['gce-xfstests', '--instance-name', self.instance_name]
    if gce_zone:
      gce_xfstests_cmd.extend(['--gce-zone', gce_zone])
    if gs_bucket:
      gce_xfstests_cmd.extend(['--gs-bucket', gs_bucket])
    if bucket_subdir:
      gce_xfstests_cmd.extend(['--bucket-subdir', bucket_subdir])
    gce_xfstests_cmd.extend(['--image-project', self.gce_project])
    gce_xfstests_cmd.extend(self.config_cmd_arr)
    gce_xfstests_cmd.extend(self.extra_cmd_arr)
    self.gce_xfstests_cmd = gce_xfstests_cmd

    # LOG/RESULTS VARIABLES
    self.log_file_path = log_dir_path + self.id
    self.cmdlog_file_path = self.log_file_path + '.cmdlog'
    self.serial_output_file_path = self.log_file_path + '.serial'
    self.results_name = '%s-%s-%s' % (LTM.ltm_username, self.test_run_id,
                                      self.id)
    self.tmp_results_dir = '/tmp/results-%s-%s-%s' % (
        LTM.ltm_username, self.test_run_id, self.id)
    self.unpacked_results_dir = '%s/results-%s-%s-%s' % (
        log_dir_path, LTM.ltm_username, self.test_run_id, self.id)
    self.unpacked_results_serial = self.unpacked_results_dir+'.serial'

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
    started = self.__start()
    if not started:
      logging.error('Shard %s failed to start', self.id)
      logging.error('Command was %s', str(self.gce_xfstests_cmd))
    else:
      successful = self.__monitor()
      self.__finish(successful)
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
    return returned == 0

  def _update_serial_data(self, last_serial_port):
    """Update locally stored serial output with data since last serial port.

    Args:
      last_serial_port: the start byte offset to pass to the compute engine
                        API.
    Returns:
      int: The byte offset that this update call went up to. The next
           call to update_serial_data should use this int as the argument.
    """
    serial_data = self.compute.instances().getSerialPortOutput(
        project=self.gce_project, zone=self.gce_zone,
        instance=self.instance_name, start=last_serial_port).execute()
    if int(serial_data['start']) > last_serial_port:
      with io.open(self.serial_output_file_path, 'a', encoding='utf-8') as sf:
        sf.write(unicode('\n!=====Missing data from %d to %s=====!\n' %
                         (last_serial_port, serial_data['start'])))
    with io.open(self.serial_output_file_path, 'a', encoding='utf-8') as sf:
      sf.write(unicode(serial_data['contents']))
    return int(serial_data['next'])

  def _shutdown_test_vm_timeout(self, metadata):
    """Initiates shutdown of test appliance after timeout.

    This sets the metadata of the VM to have a shutdown_reason of test timeout,
    and initializes the shutdown by calling instances delete on the GCE api.

    If the KEEP_DEAD_VM option is present, this function should not be called.

    Args:
      metadata: the metadata of the instance
    """
    self.vm_timed_out = True
    for i in metadata['items']:
      # if the reason is already present, the VM is shutting down.
      # No need to call delete again.
      if i['key'] == 'shutdown_reason':
        return
    metadata['items'].append({
        'key': 'shutdown_reason',
        'value': 'ltm detected test timeout'
    })
    self.compute.instances().setMetadata(
        project=self.gce_project, zone=self.gce_zone,
        instance=self.instance_name, body=metadata).execute()
    self.compute.instances().delete(
        project=self.gce_project, zone=self.gce_zone,
        instance=self.instance_name).execute()

  def __monitor(self):
    """Main monitor loop of shard process.

    This function polls the GCE api every 60 seconds for the existence of the
    test VM and its status. This also will call update_serial_data to keep
    track of all available serial port output from the test VM.

    When the test appliance is detected to have completed, it will return True.

    If the status does not change for more than an hour, the test appliance is
    assumed to have wedged, and this will return false.

    Returns:
      boolean value: True if the test VM finished. False if the test VM is
                     presumed to have wedged, and results will not be available
                     Even if the test VM finished, results might not be
                     available (i.e. if it timed out and wasn't able to even run
                     a shutdown script due to a kernel crash)
    """
    logging.info('Entered monitor.')
    logging.info('Waiting for test VM to complete...')

    # uncomment to get rid of noisy logging.
    # logging.getLogger('googleapiclient.discovery').setLevel(logging.WARNING)
    self.vm_timed_out = False
    self.compute = googleapiclient.discovery.build('compute', 'v1')
    wait_time, time_of_last_status, last_serial_port = 0, 0, 0
    last_status = ''
    while True:
      for _ in range(60):
        sleep(1.0)
      wait_time += 60
      logging.info('Querying for instance %s', self.instance_name)
      try:
        last_serial_port = self._update_serial_data(last_serial_port)
        # Check status of test appliance.
        instance_info = self.compute.instances().get(
            project=self.gce_project, zone=self.gce_zone,
            instance=self.instance_name).execute()
        new_status = ''
        for m in instance_info['metadata']['items']:
          if m['key'] == 'status':
            new_status = m['value']
            break
        if new_status != last_status:
          time_of_last_status = wait_time
          last_status = new_status
        elif (new_status == last_status and
              wait_time > time_of_last_status + 3600):
          logging.info('Instance seems to have wedged, no status update '
                       'for >1 hour.')
          logging.info('Wait time: %d. time of last status: %d',
                       wait_time, time_of_last_status)
          logging.info('Last seen status was: %s', last_status)
          if self.keep_dead_vm:
            return False
          else:
            metadata = instance_info['metadata']
            self._shutdown_test_vm_timeout(metadata)
      except googleapiclient.errors.HttpError as e:
        logging.info('Got error %s', e)
        if 'not found' in str(e) and '404' in str(e):
          logging.info('Test VM no longer exists!')
          break
    return True

  def __finish(self, successful):
    """Finds and downloads results, or dumps serial output.

    Get the results from the GS bucket if it can be found there.
    If it is not found, use the serial port output in place of results.
    If it is found, download it to a local directory, and delete it from the
    GS bucket. Also delete the serial port output, as it won't be needed.

    Args:
      successful: whether or not the monitor loop had succeeded or failed.
    """
    logging.info('Getting results file from gcs bucket')

    sc = storage.Client()
    self.bucket = sc.lookup_bucket(self.gs_bucket)
    if not successful:
      self._error_finish()
      return

    results_url = self._get_results_url()
    if not results_url:
      logging.warning('Could not find results url')
      return self._error_finish()

    cmdf = open(self.cmdlog_file_path, 'a')

    logging.info('Calling get-results')
    resultsval = call(
        ['gce-xfstests', 'get-results', '--unpack', results_url],
        stdout=cmdf)
    cmdf.close()

    # unpack results dir.
    if resultsval != 0 and not os.path.isdir(self.tmp_results_dir):
      logging.warning('error occurred, can\'t find unpacked results')
      logging.warning('gce-xfstests get-results returned %d', resultsval)
      return self._error_finish()
    else:
      shutil.move(self.tmp_results_dir, self.unpacked_results_dir)

    if os.path.isfile(self.serial_output_file_path) and not self.vm_timed_out:
      # If the vm timed out, we should still keep the serial output, despite
      # having a results file. This may be helpful for diagnosis (e.g. if the VM
      # was actually still alive and well, but tests stopped progressing for
      # whatever reason)
      os.remove(self.serial_output_file_path)
    logging.info('Removing shard %s results files from gcs', self.id)
    bucket_subdir = 'results'
    if self.bucket_subdir:
      bucket_subdir = self.bucket_subdir
    for b in self.bucket.list_blobs(prefix='%s/results.%s' %
                                    (bucket_subdir, self.results_name)):
      b.delete()
    for b in self.bucket.list_blobs(prefix='%s/summary.%s' %
                                    (bucket_subdir, self.results_name)):
      b.delete()

    logging.info('Finished')
    return

  def _error_finish(self):
    logging.info('Finishing with errors. Regular results will not be '
                 'available for this shard.')
    try:
      # copy serial output to temp directory
      shutil.move(self.serial_output_file_path, self.unpacked_results_serial)
      logging.info('Dumped serial port output to results')
    except IOError:
      logging.warning('No results or serial port output available!')
    return

  def _get_results_url(self):
    """Get the GS results URI.

    This function waits for up to 20 seconds for the results tarball to appear
    in the GS bucket, polling for it every 5 seconds.

    Returns:
      The GS results uri for this shard, or None if the tarball could not be
      found for 20 minutes.

    """
    tries = 0
    while tries < 5:
      logging.info('Checking if results.%s exists, try %d',
                   self.results_name, tries)
      # list_blobs returns an iterable.
      bucket_subdir = 'results'
      if self.bucket_subdir:
        bucket_subdir = self.bucket_subdir
      for b in self.bucket.list_blobs(prefix='%s/results.%s' %
                                      (bucket_subdir, self.results_name)):
        logging.info('Found blob with name %s', b.name)
        return 'gs://%s/%s' % (self.gs_bucket, b.name)
      sleep(5.0)
      tries += 1
    return None

### end class Shard

