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
