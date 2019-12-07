"""This module is for authentication for the build server.

This module, on first boot, assumes that a password file, 'ltm-pass', is
available in the GS bucket for downloading.
After initialization, a .user.json file will be created in the directory
of the build server, which will contain the password hash, the
username, and a salt.
"""
import binascii
import hashlib
import json
import random
import string
import flask_login
import gce_funcs
import google
from google.cloud import storage


def random_string(size=15, chars=string.ascii_uppercase + string.digits):
  return ''.join(random.SystemRandom().choice(chars) for _ in range(size))


class User(flask_login.UserMixin):
  """User subclass for flask_login.

  The data for the single user is stored with the scripts at ./.user.json. At
  initialization, the file will not exist, so the User will look for a
  ltm-pass file in the root of the GS bucket, and randomly generate a username
  and salt.
  Whenever a User object is instantiated after this, it will read the data from
  this file.
  """

  user_data_file_path = '/usr/local/lib/gce-bldsrv/.user.json'

  def __init__(self):
    f = open(User.user_data_file_path, 'r')
    self.user_vals = json.loads(f.read())
    f.close()
    self.username = self.user_vals['username']
    self.id = self.username
    self.hashed_password = self.user_vals['password']
    self.salt = self.user_vals['salt']

  @property
  def is_authenticated(self):
    # This property is used by Flask-login after attempting to load in a User
    # object from the Flask session cookies.
    # If the requester has a valid session cookie, this User object is loaded.
    # If not, the AnonymousUserMixin object is loaded, and on that class this
    # property is set to False.
    return True

  @property
  def is_anonymous(self):
    return False

  @property
  def is_active(self):
    return True

  @staticmethod
  def create_user():
    if User.check_user_file():
      return User()

    # attempt to fetch password from GCS.
    storage_client = storage.Client()
    bucket_name = gce_funcs.get_gs_bucket().strip()
    bucket = storage_client.lookup_bucket(bucket_name)

    try:
      newpass = bucket.blob('ltm-pass').download_as_string().strip()
    except google.cloud.exceptions.NotFound as e:
      # need to error here. If this happens the user managed to delete
      # gs://$GS_BUCKET/ltm-pass between launch-ltm and the boot of the
      # webserver.
      print 'Could not find password.'
      raise e

    newusername = random_string(size=8)
    newsalt = random_string(size=20)

    # store the SHA512 hash in hex on the management server
    hashed_password = get_password_hash(newpass, newsalt)
    newuser = {
        'username': newusername,
        'password': hashed_password,
        'salt': newsalt
    }

    with open(User.user_data_file_path, 'w') as f:
      json.dump(newuser, f)
    return User()

  @staticmethod
  def check_user_file():
    try:
      with open(User.user_data_file_path, 'r') as f:
        user_vals = json.loads(f.read())
        if ('username' in user_vals and
            'password' in user_vals and
            'salt' in user_vals):
          return True
    except IOError:
      pass
    return False

  @staticmethod
  def get(user_id):
    user = User()
    # If multiple users were to exist, we could construct a User
    # object from a database using the user_id to query the
    # database. But there's only one user.
    if user.username == user_id:
      return user
    else:
      return None

  def validate_password(self, password):
    hashed_password = get_password_hash(password, self.salt)
    return self.hashed_password == hashed_password


def get_password_hash(password, salt):
  return binascii.hexlify(
      hashlib.pbkdf2_hmac('sha512', password, salt, 234567))
