#!/usr/bin/env python3
"""A stand-in for the gce-xfstests LTM/KCS server, for local testing.

It mimics the parts of gce-server/util/server/server.go which the
gce-xfstests client talks to: an HTTPS server with a self-signed,
CN-only certificate, a /login endpoint which sets a session cookie,
and /status and /gce-xfstests endpoints which require that cookie.

Every request received is appended as a JSON line to --log so that
tests can check exactly what (and how many times) the server was asked
to do something.
"""

import argparse
import http.server
import json
import socketserver
import ssl
import sys
import time

PASSWORD = "sekrit-pw"
TOKEN = "MTcZTOKENabcDEF=="


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server_version = "fake-ltm"

    def log_message(self, fmt, *args):
        pass

    def _record(self, body):
        rec = {
            "time": time.time(),
            "method": self.command,
            "path": self.path,
            "headers": {k.lower(): v for k, v in self.headers.items()},
            "body": body.decode("utf-8", "replace"),
        }
        with open(self.server.logpath, "a") as f:
            f.write(json.dumps(rec) + "\n")

    def _reply(self, status, body, ctype="application/json", extra=()):
        if isinstance(body, str):
            body = body.encode()
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        for k, v in extra:
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _logged_in(self):
        cookie = self.headers.get("Cookie", "")
        for item in cookie.split(";"):
            if item.strip() == "single-session=" + TOKEN:
                return True
        return False

    def do_GET(self):
        self._record(b"")
        if self.path.split("?")[0] == "/":
            self._reply(200, "index page", "text/plain")
        else:
            self._reply(404, "404 page not found\n", "text/plain")

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length) if length else b""
        self._record(body)
        if self.path == "/login":
            try:
                req = json.loads(body.decode())
            except ValueError as e:
                self._reply(400, str(e) + "\n", "text/plain")
                return
            if req.get("password") != PASSWORD:
                self._reply(400, "Wrong password\n", "text/plain")
                return
            exp = time.strftime("%a, %d %b %Y %H:%M:%S GMT",
                                time.gmtime(time.time() + 2592000))
            cookie = ("single-session=%s; Path=/; Expires=%s; "
                      "Max-Age=2592000" % (TOKEN, exp))
            self._reply(200, json.dumps({"status": True,
                                         "msg": "login succeeded"}),
                        extra=[("Set-Cookie", cookie)])
            return
        delay_s = None
        if self.path.startswith("/delay/"):
            delay_s = float(self.path.split("/", 2)[2])
        if delay_s is not None or self.path in (
                "/status", "/gce-xfstests", "/slow"):
            if not self._logged_in():
                self._reply(403, "Login failed\n", "text/plain")
                return
            if delay_s is not None:
                time.sleep(delay_s)
            elif self.path == "/slow":
                time.sleep(self.server.slow)
            if self.path == "/status":
                self._reply(200, json.dumps({"sharders": [], "watchers": [],
                                             "bisectors": []}))
                return
            try:
                req = json.loads(body.decode())
            except ValueError:
                self._reply(500, "Failed to parse json request\n",
                            "text/plain")
                return
            self._reply(200, json.dumps({
                "status": True,
                "testID": "20261120000000",
                "msg": "Launching tests",
                "echo": req.get("orig_cmdline"),
            }))
            return
        self._reply(404, "404 page not found\n", "text/plain")


class Server(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True
    allow_reuse_address = True


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=0)
    ap.add_argument("--cert", required=True)
    ap.add_argument("--key", required=True)
    ap.add_argument("--log", required=True)
    ap.add_argument("--portfile")
    ap.add_argument("--slow", type=float, default=5.0)
    args = ap.parse_args()

    srv = Server(("127.0.0.1", args.port), Handler)
    srv.logpath = args.log
    srv.slow = args.slow
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.load_cert_chain(args.cert, args.key)
    srv.socket = ctx.wrap_socket(srv.socket, server_side=True,
                                 do_handshake_on_connect=False)
    if args.portfile:
        with open(args.portfile, "w") as f:
            f.write(str(srv.server_address[1]))
    sys.stderr.write("fake_ltm listening on %d\n" % srv.server_address[1])
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
