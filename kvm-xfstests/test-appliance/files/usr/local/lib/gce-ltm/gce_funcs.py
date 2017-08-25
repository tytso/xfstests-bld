"""GCE LTM library functions used across multiple other modules.

Library funcs to parse and read out variables used in the test appliance shell
scripts, such as the gs bucket variable, and variables that come from the
config file downloaded on boot. Shell scripts that fetch metadata write the
value to a local location (GCE_STATE_DIR) to prevent repeated networking, so
the functions here do the same.
"""
import requests

GCE_STATE_DIR = '/var/lib/gce-xfstests/'
GCE_META_URL = 'http://metadata.google.internal/computeMetadata/v1/instance/'
GCE_PROJ_URL = 'http://metadata.google.internal/computeMetadata/v1/project/'
GCE_CONFIG_FILE = '/root/xfstests_bld/kvm-xfstests/config.gce'

GC_META_HEADERS = {'Metadata-Flavor': 'Google'}


def get_metadata_value(file_name, metadata_name, project=False):
  """Gets GCE metadata values from either local cache or internal request.

  This first tries to access the file file_name, but if it encounters an
  IOError writing the file, it will make a request to the metadata server.
  If the request succeeds, the metadata value will be written to file_name
  which is effectively used as a cache.
  Args:
    file_name: name of file under GCE_STATE_DIR to look for cached result
    metadata_name: uri path from v1/instance/ or v1/project/ to the metadata
                   value
    project: flag to check the project metadata instead of the instance
             metadata
  Returns:
    String corresponding to the metadata value specified. If the metadata
    could not be fetched, an empty string will be returned.
  """
  try:
    f = open(GCE_STATE_DIR + file_name, 'r')
    metadata_value = f.read()
    f.close()
  except IOError:
    req_url = GCE_META_URL
    if project:
      req_url = GCE_PROJ_URL
    r = requests.get(req_url + metadata_name,
                     headers=GC_META_HEADERS)
    if r.status_code >= 400:
      metadata_value = ''
    else:
      metadata_value = r.text.strip()
      write_metadata_value_local(file_name, metadata_value)
  return metadata_value


def write_metadata_value_local(file_name, str_val):
  try:
    f = open(GCE_STATE_DIR + file_name, 'w')
    f.write(str_val)
    f.close()
  except IOError:
    print 'something went wrong'
    return False
  return True


def get_gs_bucket():
  gs_bucket = get_metadata_value('gs_bucket', 'attributes/gs_bucket').strip()
  return gs_bucket


def get_gs_id():
  gs_id = get_metadata_value('gce_id', 'id').strip()
  return gs_id


def get_gce_zone():
  full_zone = get_metadata_value('gce_zone', 'zone')
  base_zone = full_zone.split('/')[-1].strip()
  return base_zone


def get_proj_id():
  proj_name = get_metadata_value('gce_proj_name',
                                 'project-id', project=True).strip()
  return proj_name


class GCEConfig(object):
  upload_summary = 'GCE_UPLOAD_SUMMARY'
  bucket_subdir = 'BUCKET_SUBDIR'
  min_scratch_size = 'GCE_MIN_SCR_SIZE'
  keep_dead_vm = 'GCE_LTM_KEEP_DEAD_VM'
  sendgrid_api_key = 'SENDGRID_API_KEY'
  report_sender = 'GCE_REPORT_SENDER'


def get_config():
  """Get the gce_xfstests config as a dictionary.

  Parses the bash 'declare -p' syntax in the config file, getting variables
  without having to source the file or check the environment variables.

  Returns:
    config: a dictionary containing all parsed environment variables.
  """
  # Config file looks like
  # declare -- VARNAME="VALUE"
  config = {}
  try:
    with open(GCE_CONFIG_FILE, 'r') as f:
      for line in f:
        try:
          k, v = line.split('=')
          k = k.split(' ')[-1]  # get rid of "declare -- k"
          v = v.strip('\"\n')  # get rid of the extraneous quotes.
          config[k] = v
        except (ValueError, IndexError):
          pass
  except IOError:
    pass  # if the file isn't there, we just return an empty config.
  return config


def get_upload_summary():
  config = get_config()
  # needs to be a non-zero string.
  return GCEConfig.upload_summary in config and config[GCEConfig.upload_summary]


def get_bucket_subdir():
  config = get_config()
  if GCEConfig.bucket_subdir in config:
    return config[GCEConfig.bucket_subdir]
  else:
    return ''


def get_min_scratch_size():
  config = get_config()
  try:
    if GCEConfig.min_scratch_size in config:
      return int(config[GCEConfig.min_scratch_size])
  except ValueError:
    pass
  return 0


def get_keep_dead_vm():
  config = get_config()
  if GCEConfig.keep_dead_vm in config:
    return config[GCEConfig.keep_dead_vm]
  else:
    return ''


def get_sendgrid_api_key():
  config = get_config()
  if GCEConfig.sendgrid_api_key in config:
    return config[GCEConfig.sendgrid_api_key]
  else:
    return ''


def get_email_report_sender():
  config = get_config()
  if GCEConfig.report_sender in config:
    return config[GCEConfig.report_sender]
  else:
    return ''
