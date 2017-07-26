#!/usr/bin/python
"""Main webserver script of the gce-xfstests LTM.

This script utilizes the flask web framework to respond to all URLs not
directly served by the lighttpd server.

This is run from the main fcgi executable. The return values here are
served back to the lighttpd server over the FCGI socket.

Main endpoints are:
  /login - to authenticate, post only as this is intended to be done by a
  command line script

  /gce-xfstests - full gce-xfstests command line in b64 can be passed via
  JSON in post data, and the LTM will run the test. The command itself
  is passed into a TestRunManager object which manages the test run.

All of the logging done in the server process is sent to the file
"/var/log/lgtm/lgtm.log".
"""
import binascii
import logging
import os
import flask
from ltm import LTM

logging.basicConfig(
    filename=LTM.server_log_file,
    format='[%(levelname)s:%(asctime)s '
    '%(filename)s:%(lineno)s-%(funcName)s()] %(message)s',
    datefmt='%Y-%m-%d %H:%m:%S', level=logging.DEBUG)

app = flask.Flask(__name__, static_url_path='/static')

# The secret key is used by Flask as a server-side secret to prevent tampering
# of session cookies (for authentication). Flask requires that the secret be
# set if sessions are used.
# The LTM is not concerned with long user sessions and isn't really
# restarted regularly, so generating the key on initial setup from a regular
# test appliance is fine.
secret_key_path = '/var/www/.ltm_secret_key'
if os.path.isfile(secret_key_path):
  with open(secret_key_path, 'r') as f:
    secret_key = f.read()
else:
  with open(secret_key_path, 'w') as f:
    # slice last value off because it's a newline
    secret_key = binascii.b2a_uu(os.urandom(26))[:-1]
    f.write(secret_key)

app.secret_key = secret_key


@app.route('/')
def index():
  logging.info('Request received at /, returning index.html')
  return app.send_static_file('index.html')

if __name__ == '__main__':
  app.run(host='0.0.0.0')

