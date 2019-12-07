#!/usr/bin/python
'''
This script allows the lighttpd server to communicate with the python
build server scripts through the FCGI socket that lighttpd provides.

'''
from flup.server.fcgi import WSGIServer
from app import app

# when Lighttpd serves a request that isn't under [/var/www]/static/
# this fastcgi server will be invoked
if __name__ == "__main__":
  WSGIServer(app).run()
