"""Class that holds static constants for LTM server scripts."""
import os


class LTM(object):
  server_log_file = '/var/log/lgtm/lgtm.log'
  test_log_dir = '/var/log/lgtm/ltm_logs/'
  ltm_username = 'ltm'

  @staticmethod
  def create_log_dir(log_file_path):
    if not os.path.exists(os.path.dirname(log_file_path)):
      os.makedirs(os.path.dirname(log_file_path))

# end class LTM
