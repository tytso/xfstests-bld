"""Build class to build kernel from repository and commit.

Builds are created by the buildmanager.

Calling run() on this object will spawn subprocesses to clone/update the repo
and then do the build.

The first subprocess runs a shell script that clones/updates the repo.

The second subprocess runs a shell script that executes the build.

The build process then waits for the build run to complete.

When the image is ready, the shell script will upload it into GCS
before exiting the process.
"""
from datetime import datetime
import io
import logging
from multiprocessing import Process
import os
import shutil
import subprocess
from subprocess import call
import sys
from time import sleep

import gce_funcs
from bldsrv import BLDSRV
from google.cloud import storage


class Kbuild(object):
  """Build class."""

  def __init__(self, repository, commit, build_dir, build_id,
               log_dir_path):

    self.repository = repository
    self.commit = commit
    self.build_dir = build_dir
    self.id = build_id
    self.build_path = BLDSRV.repo_cache_path + self.build_dir
    self.image_file_path = self.build_path + BLDSRV.image_path
    self.build_log = self.build_path + '/build.log'
    self.gs_bucket = gce_funcs.get_gs_bucket().strip()

    # LOG/RESULTS VARIABLES
    self.log_file_path = log_dir_path + self.id
    self.buildlog_file_path = self.log_file_path + '.buildlog'

    logging.debug('Starting build %s', self.id)
  # end __init__

  def run(self):
    logging.info('Spawning child process for build %s', self.id)
    self.process = Process(target=self.__run)
    self.process.start()
    return

  def _setup_logging(self):
    logging.info('Move logging to build file %s', self.log_file_path)
    logging.getLogger().handlers = []  # clear handlers for new process
    logging.basicConfig(
        filename=self.log_file_path,
        format='[%(levelname)s:%(asctime)s %(filename)s:%(lineno)s-'
               '%(funcName)s()] %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S', level=logging.INFO)
    sys.stderr = sys.stdout = open(self.log_file_path, 'a+')

  def __run(self):
    """Main function for a build

    This function will be called in a separate running process, after
    run is called. The function makes an explicit call to exit() after
    finishing the procedure to exit the process.
    This function should not be called directly.
    """
    logging.info('Child process spawned for build %s', self.id)
    self._setup_logging()
    successful = self.__start()
    if not successful:
      logging.error('Build %s failed', self.id)
      logging.error('Build details: %s commit=%s', self.repository, self.commit)
    #else:
      #successful = self.__monitor()
      #logging.info('Exiting monitor process for build %s', self.id)
    self.__finish(successful)
    exit()

  def __start(self):
    logging.debug('opening log file %s', self.buildlog_file_path)
    f = open(self.buildlog_file_path, 'w')

    logging.info('Cloning into %s', self.repository)
    clone = subprocess.Popen([BLDSRV.gce_update_repo,
      self.repository, self.build_dir, self.commit],
      stdout=f,stderr=f)
    return_code = clone.wait()
    logging.info('%s exited with return code %s', BLDSRV.gce_update_repo, return_code)

    if return_code == 0:
        logging.info('Building %s commit=%s', self.repository, self.commit)
        build = subprocess.Popen([BLDSRV.gce_build_kernel,
          self.repository, self.commit, self.build_dir, self.gs_bucket],
          stdout=f,stderr=f)
        return_code = build.wait()
        logging.info('%s exited with return code %s', BLDSRV.gce_build_kernel, return_code)

    f.close()
    return return_code == 0

  def __monitor(self):
    """Main monitor loop of build process.

    This function looks for updates in the build log every 30 seconds.
    When the build is detected to have completed, it will return True.
    If the build is not complete after 20 minutes, this will return False.

    Returns:
      boolean value: True if the build finished. False if not.
    """
    logging.info('Entered monitor.')
    logging.info('Waiting for build to complete...')

    wait_time, last_modified = 0, 0
    while True:
      for _ in range(30):
        sleep(1.0)
      wait_time += 30
      time_stamp = os.stat(self.build_log).st_mtime
      time_stamp_str = datetime.fromtimestamp(time_stamp).strftime('%H:%M:%S')
      logging.info('Querying build %s - log last modified: %s', self.id, time_stamp_str)
      if last_modified != time_stamp:
          last_modified = time_stamp
      else:
        logging.info('Build %s stopped', self.id)
        break
      if wait_time == 1200:
        logging.info('Build %s timed out', self.id)
        break

    for _ in range(60):
        sleep(1.0)
    if not os.path.isfile(self.image_file_path):
        logging.info('Build %s failed', self.id)
    else:
        logging.info('Build %s completed', self.id)

    return True

  def __finish(self, successful):
    """
    Args:
      successful: whether or not the build has succeeded.
    """
    if not successful:
        logging.info('Build failed: check %s for more details', self.build_log)
    logging.info('Finished')
    return


### end class Build
