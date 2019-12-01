"""Class that holds static constants for build server scripts."""
import os


class BLDSRV(object):
  server_log_file = '/var/log/bldsrv/bldsrv.log'
  build_log_dir = '/var/log/bldsrv/bldsrv_logs/'
  bldsrv_username = 'bldsrv'
  image_path = '/arch/x86/boot/bzImage'
  image_name = 'bzImage'
  repo_cache_path = '/repositories/'
  gce_update_repo = '/usr/local/lib/gce-update-repo'
  gce_build_kernel = '/usr/local/lib/gce-build-kernel'

  @staticmethod
  def create_log_dir(log_file_path):
    if not os.path.exists(os.path.dirname(log_file_path)):
      os.makedirs(os.path.dirname(log_file_path))

BLDSRV.create_log_dir(BLDSRV.build_log_dir)
# end class BLDSRV
