#!/usr/bin/env python3
"""A stand-in for "ssh -N -L <unix socket>:<host>:<port> ..." for tests.

It finds the -L option in its arguments, listens on the Unix-domain
socket named there, and forwards each connection to a TCP port on
localhost, just like OpenSSH's local forwarding would (except that
there is no remote machine).

Behaviour knobs (environment variables):
  FAKE_SSH_PORT        forward to this port instead of the one in -L
  FAKE_SSH_DELAY       seconds to wait before creating the socket
                       (simulates authentication time)
  FAKE_SSH_FAIL        print this message and exit 255 instead
  FAKE_SSH_DIE_AFTER   exit 255 this many seconds after starting
                       (simulates a broken connection)
  FAKE_SSH_ARGV_LOG    append the argument list (JSON) to this file
  FAKE_SSH_PID_LOG     append our pid to this file
"""

import json
import os
import signal
import socket
import sys
import threading
import time


def pump(src, dst):
    try:
        while True:
            data = src.recv(65536)
            if not data:
                break
            dst.sendall(data)
    except OSError:
        pass
    finally:
        try:
            dst.shutdown(socket.SHUT_WR)
        except OSError:
            pass


def handle(conn, port):
    try:
        up = socket.create_connection(("127.0.0.1", port))
    except OSError as e:
        sys.stderr.write("channel 2: open failed: connect failed: %s\n" % e)
        sys.stderr.flush()
        conn.close()
        return
    t = threading.Thread(target=pump, args=(up, conn), daemon=True)
    t.start()
    pump(conn, up)
    t.join()
    conn.close()
    up.close()


def main():
    argv = sys.argv[1:]
    if os.environ.get("FAKE_SSH_ARGV_LOG"):
        with open(os.environ["FAKE_SSH_ARGV_LOG"], "a") as f:
            f.write(json.dumps(argv) + "\n")
    if os.environ.get("FAKE_SSH_PID_LOG"):
        with open(os.environ["FAKE_SSH_PID_LOG"], "a") as f:
            f.write("%d\n" % os.getpid())
    if os.environ.get("FAKE_SSH_FAIL"):
        sys.stderr.write(os.environ["FAKE_SSH_FAIL"] + "\n")
        sys.exit(255)

    spec = None
    for i, a in enumerate(argv):
        if a == "-L" and i + 1 < len(argv):
            spec = argv[i + 1]
        elif a.startswith("-L") and len(a) > 2:
            spec = a[2:]
    if not spec:
        sys.stderr.write("fake_ssh: no -L option\n")
        sys.exit(255)
    path, host, port = spec.rsplit(":", 2)
    port = int(os.environ.get("FAKE_SSH_PORT") or port)

    signal.signal(signal.SIGTERM, lambda *a: os._exit(143))
    time.sleep(float(os.environ.get("FAKE_SSH_DELAY") or 0))

    die_after = os.environ.get("FAKE_SSH_DIE_AFTER")
    if die_after:
        def die():
            sys.stderr.write("client_loop: send disconnect: Broken pipe\n")
            sys.stderr.flush()
            os._exit(255)
        threading.Timer(float(die_after), die).start()

    if os.path.exists(path):
        os.unlink(path)
    old = os.umask(0o177)
    srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    srv.bind(path)
    os.umask(old)
    srv.listen(16)
    while True:
        conn, _ = srv.accept()
        threading.Thread(target=handle, args=(conn, port),
                         daemon=True).start()


if __name__ == "__main__":
    main()
