#!/usr/bin/env python3
"""End-to-end tests for run-fstests/util/gce-ssh-curl.

Everything runs locally, against loopback servers and fakes:

  fake_ltm.py   stands in for the LTM/KCS server (HTTPS, CN-only cert)
  fake_ssh.py   stands in for ssh in most tests (provides -L Unix-socket
                forwarding to localhost, with knobs for delays/failures)
  sshd          a real OpenSSH server (run as the current user on an
                ephemeral loopback port) and the real ssh client, used
                in the real_ssh test when OpenSSH is installed

What is real: run-fstests/util/gce-ssh-curl (both the curl wrapper and
its background --tunnel-daemon), curl, jq, and the shell functions in
run-fstests/util (run_gce_curl is extracted from get-config;
gce-ltm-funcs and gce-kcs-funcs are sourced as they are).

Usage: test_ssh_curl.py [-v] [-k SUBSTRING] [--list] [--keep] [--repo PATH]
"""

import argparse
import base64
import json
import os
import pwd
import shlex
import shutil
import signal
import socket
import stat
import subprocess
import sys
import tempfile
import time
import traceback

HELPER_DIR = os.path.dirname(os.path.realpath(__file__))
DEFAULT_REPO = os.path.dirname(os.path.dirname(HELPER_DIR))
REPO = DEFAULT_REPO
SSH_CURL = ""
HOST = "a.test-proj.gce-xfstests"

verbose = False


class Failure(Exception):
    pass


class Skip(Exception):
    pass


def check(cond, msg):
    if not cond:
        raise Failure(msg)


def wait_for(fn, timeout=10, what="condition"):
    end = time.time() + timeout
    while time.time() < end:
        v = fn()
        if v:
            return v
        time.sleep(0.05)
    raise Failure("timed out waiting for " + what)


def resolve_repo(repo_path):
    """Locate the xfstests-bld repository root and targets under test."""
    global REPO, SSH_CURL
    REPO = os.path.realpath(repo_path)
    SSH_CURL = os.path.join(REPO, "run-fstests", "util", "gce-ssh-curl")
    missing = [p for p in (
        SSH_CURL,
        os.path.join(REPO, "run-fstests", "util", "get-config"),
        os.path.join(REPO, "run-fstests", "util", "gce-ltm-funcs"),
        os.path.join(REPO, "run-fstests", "util", "gce-kcs-funcs"),
    ) if not os.path.isfile(p)]
    if missing:
        raise SystemExit(
            "cannot find expected xfstests-bld files under %s:\n  %s"
            % (REPO, "\n  ".join(missing)))
    helper_missing = [p for p in (
        os.path.join(HELPER_DIR, "fake_ltm.py"),
        os.path.join(HELPER_DIR, "fake_ssh.py"),
    ) if not os.path.isfile(p)]
    if helper_missing:
        raise SystemExit(
            "cannot find expected test helper files under %s:\n  %s"
            % (HELPER_DIR, "\n  ".join(helper_missing)))


def require_tools():
    """Return resolved paths for the external tools used by this suite."""
    tools = {}
    for name in ("bash", "curl", "jq", "openssl"):
        path = shutil.which(name)
        if not path:
            raise SystemExit("required tool not found in PATH: %s" % name)
        tools[name] = path
    for name in ("ssh", "ssh-keygen"):
        tools[name] = shutil.which(name)
    sbin_path = os.pathsep.join([
        os.environ.get("PATH", ""),
        "/usr/sbin", "/sbin", "/usr/local/sbin",
    ])
    sshd = shutil.which("sshd", path=sbin_path)
    tools["sshd"] = os.path.abspath(sshd) if sshd else None
    return tools


def unused_tcp_port():
    """Return a loopback TCP port that currently has no listener."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def generate_self_signed_cert(cert_path, key_path, cn, openssl_bin):
    """Create an ephemeral CN-only self-signed cert (matching gce-setup-cert)."""
    cmd = [
        openssl_bin, "req", "-x509", "-newkey", "rsa:2048",
        "-keyout", key_path, "-nodes", "-out", cert_path,
        "-days", "1", "-subj", "/CN=" + cn,
    ]
    res = subprocess.run(cmd, stdin=subprocess.DEVNULL,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if res.returncode != 0:
        raise SystemExit("openssl failed generating test cert:\n%s"
                         % res.stderr.decode("utf-8", "replace"))


def proc_stat_after_comm(pid):
    """Return the fields after '(comm)' in /proc/<pid>/stat, or None."""
    try:
        with open("/proc/%d/stat" % pid, "rb") as f:
            data = f.read()
    except OSError:
        return None
    i = data.rfind(b")")
    if i < 0:
        return None
    return data[i + 2:].decode("ascii", "replace").split()


def alive(pid):
    try:
        os.kill(pid, 0)
    except OSError:
        return False
    fields = proc_stat_after_comm(pid)
    return fields is not None and fields[0] != "Z"


def child_pids(ppid):
    """Return direct non-zombie child PIDs of ppid via /proc."""
    out = []
    for entry in os.listdir("/proc"):
        if not entry.isdigit():
            continue
        pid = int(entry)
        fields = proc_stat_after_comm(pid)
        if fields and int(fields[1]) == ppid and fields[0] != "Z":
            out.append(pid)
    return out


def kill_tree(pid):
    """Recursively SIGKILL pid's descendants and then pid itself."""
    for child in child_pids(pid):
        kill_tree(child)
    try:
        os.kill(pid, signal.SIGKILL)
    except OSError:
        pass


def fmt_escape(s):
    """Escape braces so str.format() leaves literal paths untouched."""
    return s.replace("{", "{{").replace("}", "}}")


class Env:
    def __init__(self, tools):
        self.tools = tools
        # Unix-domain socket paths are limited to 108 bytes (gce-ssh-curl
        # enforces len(base) <= 90 so ".<pid>.fwd" fits).  Use /tmp if the
        # ambient TMPDIR would make socket names too long.
        tmp_root = tempfile.gettempdir()
        if len(os.fsencode(tmp_root)) > 40 or not os.path.isdir(tmp_root):
            tmp_root = "/tmp"
        self.tmp = tempfile.mkdtemp(prefix="sshcurl.", dir=tmp_root)
        os.chmod(self.tmp, 0o700)
        self.procs = []
        self.sshd = None
        self.env = dict(os.environ)
        for k in list(self.env):
            if (k.lower() in ("http_proxy", "https_proxy", "no_proxy",
                              "all_proxy")
                    or k.startswith("SSH_CURL_")
                    or k in ("XDG_RUNTIME_DIR", "GCE_CURL",
                             "XFSTESTS_CONFIG", "GCE_INSTANCE", "GCE_ZONE",
                             "GCE_PROJECT", "GCE_ACCOUNT", "GS_BUCKET")):
                del self.env[k]
        # Avoid writing __pycache__ inside the git working tree.
        self.env["PYTHONDONTWRITEBYTECODE"] = "1"

        self.curlbin = os.path.join(self.tmp, "curlbin")
        self.bin = os.path.join(self.tmp, "bin")
        self.rundir = os.path.join(self.tmp, "run-fstests")
        self.pkidir = os.path.join(self.tmp, "pki")
        self.sockdir = os.path.join(self.tmp, "s")
        for d in (self.curlbin, self.bin, self.rundir, self.pkidir):
            os.mkdir(d, 0o755)
        os.chmod(self.pkidir, 0o700)
        os.symlink(os.path.join(REPO, "run-fstests", "util"),
                   os.path.join(self.rundir, "util"))

        self.cert_path = os.path.join(self.pkidir, "cert.pem")
        self.key_path = os.path.join(self.pkidir, "key.pem")
        generate_self_signed_cert(
            self.cert_path, self.key_path, "*.test-proj.gce-xfstests",
            tools["openssl"])

        self.ltm_log = os.path.join(self.tmp, "ltm.log")
        self.pid_log = os.path.join(self.tmp, "ssh.pids")
        self.argv_log = os.path.join(self.tmp, "ssh.argv")
        open(self.ltm_log, "w").close()
        self.ltm_port = self.start_server(
            [sys.executable, os.path.join(HELPER_DIR, "fake_ltm.py"),
             "--cert", self.cert_path, "--key", self.key_path,
             "--log", self.ltm_log, "--slow", "4"], "ltm")
        # A curl wrapper which directs connections to the fake LTM server
        # (the real server listens on port 443).
        self.script(
            os.path.join(self.curlbin, "curl"),
            'exec %s --connect-to "::127.0.0.1:%d" "$@"\n'
            % (shlex.quote(tools["curl"]), self.ltm_port))

        fake_ssh = os.path.join(HELPER_DIR, "fake_ssh.py")
        self.env.update({
            "HOME": self.tmp,
            "SSH_CURL_DIR": self.sockdir,
            "SSH_CURL_SSH": "%s %s {user}@{instance}.{zone}.{project_dns}" % (
                fmt_escape(shlex.quote(sys.executable)),
                fmt_escape(shlex.quote(fake_ssh))),
            "FAKE_SSH_PORT": str(self.ltm_port),
            "FAKE_SSH_PID_LOG": self.pid_log,
            "FAKE_SSH_ARGV_LOG": self.argv_log,
        })
        self.gce = {
            "GCE_INSTANCE": "xfstests-ltm",
            "GCE_ZONE": "us-central1-c",
            "GCE_PROJECT": "test-proj",
        }
        for kind in ("ltm", "kcs"):
            with open(os.path.join(self.rundir,
                                   ".%s_instance_test-proj" % kind),
                      "w") as f:
                K = kind.upper()
                f.write("GCE_%s_SERVER_CERT=%s\n"
                        % (K, shlex.quote(self.cert_path)))
                f.write("GCE_%s_NAME=xfstests-%s\n" % (K, kind))
                f.write("GCE_%s_EXT_IP=10.11.12.13\n" % K)
                f.write("GCE_%s_PWD=sekrit-pw\n" % K)
        self.prelude = """
DIR=%(rundir)s
GCE_PROJECT=test-proj
GCE_ZONE=us-central1-c
GS_BUCKET=test-bucket
GCE_ACCOUNT=
eval "$(sed -n '/^    function run_gce_curl ()/,/^    }/p' \\
        %(get_config)s)"
function run_gcloud () { false; }
""" % {"rundir": shlex.quote(self.rundir),
       "get_config": shlex.quote(
           os.path.join(REPO, "run-fstests", "util", "get-config"))}

    def script(self, path, body):
        with open(path, "w") as f:
            f.write("#!/bin/sh\n" + body)
        os.chmod(path, 0o755)

    def start_server(self, cmd, name):
        portfile = os.path.join(self.tmp, name + ".port")
        log = open(os.path.join(self.tmp, name + ".err"), "w")
        p = subprocess.Popen(cmd + ["--portfile", portfile], env=self.env,
                             stdout=log, stderr=log,
                             stdin=subprocess.DEVNULL)
        self.procs.append(p)
        wait_for(lambda: os.path.exists(portfile) and
                 os.path.getsize(portfile) > 0, what=name + " to start")
        return int(open(portfile).read())

    def start_sshd(self):
        """Run a real sshd as the current user; return the ssh command."""
        missing = [k for k in ("ssh", "ssh-keygen", "sshd")
                   if not self.tools.get(k)]
        if missing:
            raise Skip("OpenSSH tools not installed: " + ", ".join(missing))
        d = os.path.join(self.tmp, "sshd")
        os.mkdir(d, 0o700)
        hostkey = os.path.join(d, "hostkey")
        userkey = os.path.join(d, "userkey")
        authkeys = os.path.join(d, "authorized_keys")
        pidfile = os.path.join(d, "sshd.pid")
        cfgfile = os.path.join(d, "sshd_config")
        logfile = os.path.join(d, "sshd.log")
        known_hosts = os.path.join(d, "known_hosts")

        for key in (hostkey, userkey):
            subprocess.check_call([self.tools["ssh-keygen"], "-q", "-t",
                                   "ed25519", "-N", "", "-f", key])
        shutil.copy(userkey + ".pub", authkeys)
        os.chmod(authkeys, 0o600)
        port = unused_tcp_port()
        with open(cfgfile, "w") as f:
            f.write('Port %d\n'
                    'ListenAddress 127.0.0.1\n'
                    'HostKey "%s"\n'
                    'PidFile "%s"\n'
                    'AuthorizedKeysFile "%s"\n'
                    'PasswordAuthentication no\n'
                    'KbdInteractiveAuthentication no\n'
                    'UsePAM no\n'
                    'StrictModes no\n'
                    'AllowTcpForwarding yes\n'
                    'LogLevel VERBOSE\n'
                    % (port, hostkey, pidfile, authkeys))
        log = open(logfile, "w")
        self.sshd = subprocess.Popen(
            [self.tools["sshd"], "-f", cfgfile, "-D", "-e"],
            stdout=log, stderr=log, stdin=subprocess.DEVNULL)
        self.procs.append(self.sshd)

        def sshd_listening():
            if self.sshd.poll() is not None:
                raise Failure("sshd exited early (%d):\n%s"
                              % (self.sshd.returncode, open(logfile).read()))
            return ("Server listening on" in open(logfile).read() or
                    (os.path.exists(pidfile) and os.path.getsize(pidfile) > 0))

        wait_for(sshd_listening, what="sshd to listen")
        user = pwd.getpwuid(os.geteuid()).pw_name
        return ("%s -F none -p %d -i %s -o IdentitiesOnly=yes "
                "-o UserKnownHostsFile=%s -o GlobalKnownHostsFile=/dev/null "
                "-o StrictHostKeyChecking=accept-new %s@127.0.0.1"
                % (fmt_escape(shlex.quote(self.tools["ssh"])), port,
                   fmt_escape(shlex.quote(userkey)),
                   fmt_escape(shlex.quote(known_hosts)),
                   fmt_escape(shlex.quote(user))))

    def run(self, args, env=None, timeout=120, gce=True, **kw):
        e = dict(self.env)
        if gce:
            e.update(self.gce)
        e.update(env or {})
        p = subprocess.run([SSH_CURL] + args, env=e,
                           stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                           stderr=subprocess.PIPE, timeout=timeout, **kw)
        out, err = p.stdout.decode(), p.stderr.decode()
        if verbose:
            sys.stderr.write("--- %r rc=%d\n--- stdout:\n%s\n--- stderr:\n%s"
                             % (args, p.returncode, out, err))
        return p.returncode, out, err

    def get(self, env=None, path="/", extra=(), **kw):
        return self.run(["-s", "--cacert", self.cert_path] +
                        list(extra) + ["https://%s%s" % (HOST, path)],
                        env, **kw)

    def bash(self, script, env=None, timeout=120):
        e = dict(self.env)
        e.update(env or {})
        p = subprocess.run([self.tools["bash"], "-c", self.prelude + script],
                           env=e, stdin=subprocess.DEVNULL,
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                           timeout=timeout)
        out, err = p.stdout.decode(), p.stderr.decode()
        if verbose:
            sys.stderr.write("--- rc=%d\n--- stdout:\n%s--- stderr:\n%s"
                             % (p.returncode, out, err))
        return p.returncode, out, err

    def ssh_pids(self):
        try:
            return [int(l) for l in open(self.pid_log) if l.strip()]
        except FileNotFoundError:
            return []

    def ssh_argv(self):
        return [json.loads(l) for l in open(self.argv_log) if l.strip()]

    def socks(self, suffix=".sock"):
        try:
            return sorted(n for n in os.listdir(self.sockdir)
                          if n.endswith(suffix))
        except FileNotFoundError:
            return []

    def daemon_pids(self, root=None):
        prefix = os.fsencode(os.path.join(root or self.sockdir, ""))
        pids = []
        for entry in os.listdir("/proc"):
            if not entry.isdigit():
                continue
            pid = int(entry)
            try:
                with open("/proc/%d/cmdline" % pid, "rb") as f:
                    argv = f.read().split(b"\0")
            except OSError:
                continue
            if b"--tunnel-daemon" not in argv:
                continue
            try:
                base = argv[argv.index(b"--base") + 1]
            except (ValueError, IndexError):
                continue
            if base.startswith(prefix) and alive(pid):
                pids.append(pid)
        return sorted(pids)

    def log_text(self):
        text = ""
        for n in self.socks(".log"):
            text += open(os.path.join(self.sockdir, n)).read()
        return text

    def ltm_requests(self):
        return [json.loads(l) for l in open(self.ltm_log) if l.strip()]

    def reset(self):
        self.run(["--tunnel-stop"], gce=False)
        for p in self.daemon_pids(self.tmp):
            try:
                os.kill(p, signal.SIGTERM)
            except OSError:
                pass
        wait_for(lambda: not self.daemon_pids(self.tmp),
                 what="daemons to exit")
        for pid in self.ssh_pids():
            wait_for(lambda: not alive(pid), what="ssh %d to exit" % pid)
        if self.sshd is not None:
            if self.sshd.poll() is None:
                self.sshd.kill()
                self.sshd.wait()
            if self.sshd in self.procs:
                self.procs.remove(self.sshd)
            self.sshd = None
        shutil.rmtree(os.path.join(self.tmp, "sshd"), ignore_errors=True)
        shutil.rmtree(self.sockdir, ignore_errors=True)
        for kind in ("ltm", "kcs"):
            try:
                os.unlink(os.path.join(self.rundir,
                                       ".%s_cookie_test-proj" % kind))
            except FileNotFoundError:
                pass
        for f in (self.ltm_log, self.pid_log, self.argv_log):
            open(f, "w").close()

    def cleanup(self, keep=False):
        try:
            self.run(["--tunnel-stop"], gce=False)
        except Exception:
            pass
        for p in self.daemon_pids(self.tmp):
            try:
                os.kill(p, signal.SIGKILL)
            except OSError:
                pass
        for p in self.ssh_pids():
            try:
                os.kill(p, signal.SIGKILL)
            except OSError:
                pass
        for p in self.procs:
            if p.poll() is None:
                p.kill()
                try:
                    p.wait(5)
                except subprocess.TimeoutExpired:
                    pass
        if keep:
            sys.stderr.write("kept " + self.tmp + "\n")
        else:
            shutil.rmtree(self.tmp, ignore_errors=True)


CMDLINE = base64.b64encode(b"ltm -c ext4/4k generic/001").decode()
LTM = ('. $DIR/util/gce-ltm-funcs\nLTM_INFO=yes send_to_ltm unused\n'
       'unset LTM_INFO\nBUCKET_SUBDIR=sub COMMIT=v6.1 send_to_ltm %s\n'
       % CMDLINE)
KCS = ('. $DIR/util/gce-kcs-funcs\n'
       'COMMIT=v6.1 GIT_REPO=https://example.com/linux.git '
       'send_to_kcs %s\n' % CMDLINE)


def strip(reqs):
    return [{"method": r["method"], "path": r["path"], "body": r["body"],
             "headers": r["headers"]} for r in reqs]


TESTS = []


def test(fn):
    TESTS.append(fn)
    return fn


@test
def ltm_kcs_flow(env):
    """The real shell functions, with GCE_CURL=gce-ssh-curl, vs plain curl"""
    e = {"PATH": env.curlbin + ":" + env.env["PATH"]}
    rc, out0, err0 = env.bash(LTM + KCS, e)
    check(rc == 0 and err0 == "", "baseline failed: %s %s" % (out0, err0))
    base = strip(env.ltm_requests())
    check([r["path"] for r in base] == ["/login", "/status", "/gce-xfstests",
                                        "/login", "/gce-xfstests"], str(base))
    env.reset()
    rc, out, err = env.bash("GCE_CURL=gce-ssh-curl\n" + LTM + KCS)
    check(rc == 0, "rc=%d %s" % (rc, err))
    check(out == out0, "output differs:\n%s\nvs\n%s" % (out, out0))
    got = strip(env.ltm_requests())
    check(got == base, "server saw\n%r\nbut with plain curl\n%r" % (got, base))
    # one tunnel each for the LTM and the KCS, each announced once
    check(err.count("gce-ssh-curl: setting up an ssh tunnel to xfstests-ltm "
                    "(us-central1-c, test-proj)") == 1 and
          err.count("gce-ssh-curl: setting up an ssh tunnel to xfstests-kcs "
                    "(us-central1-c, test-proj)") == 1 and
          len(err.strip().split("\n")) == 2, "stderr: " + err)
    check(len(env.ssh_pids()) == 2, "ssh was started %d times"
          % len(env.ssh_pids()))
    argv = env.ssh_argv()
    check(argv[0][0] == "root@xfstests-ltm.us-central1-c.test-proj" and
          argv[1][0] == "root@xfstests-kcs.us-central1-c.test-proj",
          "destinations: %r" % argv)
    fwd = argv[0][-1]
    check(argv[0][1:-1] == ["-T", "-N", "-o", "BatchMode=yes", "-o",
                            "ExitOnForwardFailure=yes", "-o",
                            "ServerAliveInterval=15", "-o",
                            "ServerAliveCountMax=3", "-o", "ControlPath=none",
                            "-L"] and fwd.endswith(":127.0.0.1:443") and
          fwd.startswith(env.sockdir + "/xfstests-ltm."), "argv: %r" % argv[0])
    # permissions
    st = os.stat(env.sockdir)
    check(stat.S_IMODE(st.st_mode) == 0o700, "dir mode %o" % st.st_mode)
    socks = env.socks()
    check(len(socks) == 2 and socks[0].startswith("xfstests-kcs.") and
          socks[1].startswith("xfstests-ltm."), "sockets: %r" % socks)
    for n in os.listdir(env.sockdir):
        st = os.stat(os.path.join(env.sockdir, n))
        mode = stat.S_IMODE(st.st_mode)
        check(st.st_uid == os.geteuid(), "owner of " + n)
        if n.endswith(".sock"):
            check(stat.S_ISSOCK(st.st_mode) and mode == 0o700,
                  "%s has mode %o" % (n, mode))
        else:
            check(mode & 0o077 == 0, "%s has mode %o" % (n, mode))
    check("sekrit" not in env.log_text() and "MTcZ" not in env.log_text(),
          "secrets in the log")


@test
def reuse_and_idle(env):
    """The tunnel is reused; the daemon exits when idle; then a new one"""
    e = {"SSH_CURL_IDLE": "2"}
    rc, out, err = env.get(e)
    check(rc == 0 and out == "index page", "%d %s %s" % (rc, out, err))
    check("setting up an ssh tunnel" in err, err)
    time.sleep(1)
    rc, out, err = env.get(e)
    check(rc == 0 and out == "index page" and err == "",
          "%d %s %s" % (rc, out, err))
    check(len(env.ssh_pids()) == 1, "tunnel was not reused")
    pid = env.ssh_pids()[0]
    dpid = env.daemon_pids()
    check(len(dpid) == 1 and alive(pid), "daemon/ssh not running")
    # idle time counts from the last use, not from the first
    time.sleep(1.5)
    check(env.daemon_pids() == dpid, "daemon exited too early")
    wait_for(lambda: not env.daemon_pids(), timeout=4, what="idle exit")
    wait_for(lambda: not alive(pid), timeout=2, what="ssh to be killed")
    check(env.socks() == [] and env.socks(".fwd") == [],
          "sockets left: %r" % os.listdir(env.sockdir))
    check("exiting: idle for 2 seconds" in env.log_text(), env.log_text())
    rc, out, err = env.get(e)
    check(rc == 0 and out == "index page" and
          "setting up an ssh tunnel" in err, "%d %s %s" % (rc, out, err))
    check(len(env.ssh_pids()) == 2, "no new tunnel")
    # SSH_CURL_VERBOSE reports tunnel reuse; a later invocation can also
    # change the idle time of a running daemon.
    rc, out, err = env.get({"SSH_CURL_IDLE": "60", "SSH_CURL_VERBOSE": "1"})
    check(rc == 0 and "using the existing ssh tunnel" in err, err)
    rc, st, err = env.run(["--tunnel-status"], gce=False)
    check("will exit after 60 seconds" in st, st)


@test
def lease_holds(env):
    """A request which takes longer than the idle time is not cut off"""
    e = dict(env.env)
    e.update(env.gce)
    e["SSH_CURL_IDLE"] = "1"
    t0 = time.time()
    p = subprocess.Popen(
        [SSH_CURL, "-s", "--cacert", env.cert_path,
         "-b", "single-session=MTcZTOKENabcDEF==",
         "-d", '{"orig_cmdline":"slow"}', "https://%s/slow" % HOST],
        env=e, stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    wait_for(lambda: env.ltm_requests(), what="slow request to reach server")
    rc_st, st, _ = env.run(["--tunnel-status"], gce=False)
    check(rc_st == 0 and "in use (" in st, st)
    out, err = p.communicate(timeout=30)
    check(p.returncode == 0 and "Launching tests" in out.decode(),
          "%d %s %s" % (p.returncode, out.decode(), err.decode()))
    check(time.time() - t0 >= 4, "not slow?")
    # ... and with an idle time of zero, each use gets its own tunnel
    # (after a grace period for whoever started the daemon)
    env.reset()
    rc, out, err = env.get({"SSH_CURL_IDLE": "0"})
    check(rc == 0 and out == "index page", "%d %s %s" % (rc, out, err))
    wait_for(lambda: not env.daemon_pids(), timeout=15, what="daemon exit")


@test
def idle_race(env):
    """Uses which arrive just as the daemon decides to exit never fail"""
    e = {"SSH_CURL_IDLE": "0.3"}
    bad = []
    for i in range(40):
        rc, out, err = env.get(e)
        if rc != 0 or out != "index page":
            bad.append((i, rc, err))
        time.sleep(0.27 + 0.004 * i)
    check(not bad, "failures: %r" % bad)
    n = len(env.ssh_pids())
    print("    (40 uses, %d tunnels)" % n, end="")
    check(1 < n < 40, "%d tunnels" % n)


@test
def ssh_dies(env):
    """The ssh connection breaks: the daemon exits; the next use recovers"""
    rc, out, err = env.get({"FAKE_SSH_DIE_AFTER": "1.5"})
    check(rc == 0 and out == "index page", "%d %s %s" % (rc, out, err))
    wait_for(lambda: not env.daemon_pids(), timeout=4, what="daemon exit")
    check("ssh: client_loop: send disconnect: Broken pipe" in env.log_text()
          and "exiting: ssh exited (status 255)" in env.log_text(),
          env.log_text())
    check(env.socks() == [] and env.socks(".fwd") == [],
          "sockets left: %r" % os.listdir(env.sockdir))
    rc, out, err = env.get()
    check(rc == 0 and out == "index page" and
          "setting up an ssh tunnel" in err, "%d %s %s" % (rc, out, err))


@test
def ssh_fails(env):
    """ssh can't log in: exit status 7, and the user is told why"""
    e = {"FAKE_SSH_FAIL": "root@host: Permission denied (publickey)."}
    t0 = time.time()
    rc, out, err = env.get(e)
    check(rc == 7 and out == "", "%d %r %s" % (rc, out, err))
    check("gce-ssh-curl: ssh: root@host: Permission denied (publickey)." in err
          and "could not set up an ssh tunnel to xfstests-ltm" in err and
          "failed (exit status 255)" in err, err)
    check(time.time() - t0 < 3, "took too long")
    wait_for(lambda: not env.daemon_pids(), timeout=3, what="daemon exit")
    check(env.socks() == [] and env.socks(".fwd") == [], "sockets left")
    check(env.ltm_requests() == [], "server was contacted")
    # In the pipeline which gce-xfstests uses, with stderr captured too
    t0 = time.time()
    rc, out, err = env.bash(
        "GCE_CURL=gce-ssh-curl\n. $DIR/util/gce-ltm-funcs\n"
        "x=$(ltm_post_json https://$LTM_HOSTNAME/ 2>&1); echo \"[$x]\"\n", e)
    check("Permission denied" in out and time.time() - t0 < 3, out + err)
    # The command can't even be run
    rc, out, err = env.get({"SSH_CURL_SSH": "/nonexistent/ssh {instance}"})
    check(rc == 7 and "cannot run /nonexistent/ssh" in err, err)
    wait_for(lambda: not env.daemon_pids(), timeout=3, what="daemon exit")


@test
def ssh_timeout(env):
    """ssh hangs: give up after SSH_CURL_TIMEOUT, and kill it"""
    t0 = time.time()
    rc, out, err = env.get({"FAKE_SSH_DELAY": "30", "SSH_CURL_TIMEOUT": "2"})
    check(rc == 7 and "timed out after 2 seconds" in err,
          "%d %r %s" % (rc, out, err))
    check(2 <= time.time() - t0 < 5, "took %.1f" % (time.time() - t0))
    pid = env.ssh_pids()[0]
    wait_for(lambda: not alive(pid), timeout=3, what="ssh to be killed")
    wait_for(lambda: not env.daemon_pids(), timeout=3, what="daemon exit")
    # Interrupting the wrapper while it waits stops the daemon as well
    e = dict(env.env)
    e.update(env.gce)
    e.update({"FAKE_SSH_DELAY": "30"})
    p = subprocess.Popen([SSH_CURL, "-s", "https://%s/" % HOST], env=e,
                         stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                         stderr=subprocess.PIPE)
    wait_for(lambda: len(env.ssh_pids()) == 2, what="ssh to start")
    p.send_signal(signal.SIGINT)
    check(p.wait(5) == 130, "exit status %r" % p.returncode)
    pid = env.ssh_pids()[1]
    wait_for(lambda: not alive(pid), timeout=5, what="ssh to be killed")
    wait_for(lambda: not env.daemon_pids(), timeout=3, what="daemon exit")


@test
def daemon_killed(env):
    """kill -9 of the daemon: ssh dies too; the stale socket is replaced"""
    rc, out, err = env.get()
    check(rc == 0, err)
    dpid = env.daemon_pids()[0]
    pid = env.ssh_pids()[0]
    os.kill(dpid, signal.SIGKILL)
    wait_for(lambda: not alive(dpid), what="daemon to die")
    wait_for(lambda: not alive(pid), timeout=3,
             what="ssh to die along with the daemon")
    check(len(env.socks()) == 1, "stale socket should still be there")
    rc, out, err = env.get()
    check(rc == 0 and out == "index page" and
          "setting up an ssh tunnel" in err, "%d %s %s" % (rc, out, err))
    check(len(env.socks()) == 1 and len(env.socks(".fwd")) == 1,
          "sockets: %r" % os.listdir(env.sockdir))


@test
def socket_removed(env):
    """Removing the socket, or SIGTERM, makes the daemon clean up and exit"""
    rc, out, err = env.get()
    check(rc == 0, err)
    pid = env.ssh_pids()[0]
    os.unlink(os.path.join(env.sockdir, env.socks()[0]))
    wait_for(lambda: not env.daemon_pids(), timeout=3, what="daemon exit")
    wait_for(lambda: not alive(pid), timeout=3, what="ssh to be killed")
    check("is gone" in env.log_text(), env.log_text())
    rc, out, err = env.get()
    check(rc == 0, err)
    pid = env.ssh_pids()[1]
    os.kill(env.daemon_pids()[0], signal.SIGTERM)
    wait_for(lambda: not env.daemon_pids(), timeout=3, what="daemon exit")
    wait_for(lambda: not alive(pid), timeout=3, what="ssh to be killed")
    check(env.socks() == [] and env.socks(".fwd") == [], "sockets left")
    check("exiting: received signal 15" in env.log_text(), env.log_text())


@test
def concurrent(env):
    """Many invocations at once, with no daemon running: one tunnel"""
    e = dict(env.env)
    e.update(env.gce)
    e["FAKE_SSH_DELAY"] = "1"
    procs = [subprocess.Popen([SSH_CURL, "-s", "--cacert",
                               env.cert_path, "https://%s/" % HOST],
                              env=e, stdin=subprocess.DEVNULL,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE)
             for _ in range(12)]
    results = [(p.wait(30), p.stdout.read(), p.stderr.read()) for p in procs]
    check(all(r[0] == 0 and r[1] == b"index page" for r in results),
          "results: %r" % results)
    check(len(env.ssh_pids()) == 1, "%d tunnels" % len(env.ssh_pids()))
    n = sum(1 for r in results if b"setting up an ssh tunnel" in r[2])
    check(n == 1, "%d invocations started a tunnel" % n)
    check(len(env.daemon_pids()) == 1, "daemons: %r" % env.daemon_pids())
    # many requests at once through the one tunnel
    procs = [subprocess.Popen([SSH_CURL, "-s", "-k", "-X", "POST", "-b",
                               "single-session=MTcZTOKENabcDEF==", "-d",
                               '{"orig_cmdline":"c%d"}' % i,
                               "https://%s/gce-xfstests" % HOST],
                              env=e, stdin=subprocess.DEVNULL,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE)
             for i in range(20)]
    for i, p in enumerate(procs):
        check(p.wait(30) == 0 and
              json.loads(p.stdout.read().decode())["echo"] == "c%d" % i,
              "request %d failed" % i)


@test
def no_fd_leak(env):
    """The daemon doesn't keep the wrapper's stdout or stderr open"""
    t0 = time.time()
    rc, out, err = env.bash(
        "export GCE_INSTANCE=xfstests-ltm GCE_ZONE GCE_PROJECT\n"
        "x=$(%s -sk https://%s/ 2>&1 | cat); echo \"[$x]\"\n"
        % (shlex.quote(SSH_CURL), HOST), {"SSH_CURL_IDLE": "20"})
    check(time.time() - t0 < 5, "took %.1f seconds" % (time.time() - t0))
    check("setting up an ssh tunnel" in out and "index page]" in out, out)
    dpid = env.daemon_pids()[0]
    fds = {}
    for fd in os.listdir("/proc/%d/fd" % dpid):
        try:
            fds[int(fd)] = os.readlink("/proc/%d/fd/%s" % (dpid, fd))
        except OSError:
            pass
    check(fds[0] == "/dev/null" and fds[1].endswith(".log") and
          fds[2].endswith(".log"), "daemon fds: %r" % fds)
    pipes = [v for v in fds.values() if v.startswith("pipe:")]
    check(len(pipes) == 1, "daemon fds: %r" % fds)   # ssh's output
    spid = env.ssh_pids()[0]
    sfd = {fd: os.readlink("/proc/%d/fd/%s" % (spid, fd))
           for fd in ("0", "1", "2")}
    check(sfd["0"] == "/dev/null" and sfd["1"] == pipes[0] and
          sfd["2"] == pipes[0], "ssh fds: %r" % sfd)
    # ssh is in its own session, with no controlling terminal
    fields = proc_stat_after_comm(spid)
    check(fields is not None and fields[3] == str(spid) and fields[4] == "0",
          "ssh stat fields: %r" % (fields,))


@test
def status_and_stop(env):
    """--tunnel-status and --tunnel-stop"""
    rc, out, err = env.run(["--tunnel-status"], gce=False)
    check(rc == 0 and "There are no ssh tunnels" in out, out + err)
    env.get()
    env.get({"GCE_INSTANCE": "xfstests-kcs"})
    rc, out, err = env.run(["--tunnel-status"], gce=False)
    check(rc == 0 and "xfstests-ltm (us-central1-c, test-proj):" in out and
          "xfstests-kcs (us-central1-c, test-proj):" in out and
          out.count("daemon pid") == 2 and "will exit after 600 seconds"
          in out and env.sockdir in out, out + err)
    pids = env.ssh_pids()
    rc, out, err = env.run(["--tunnel-stop"], gce=False)
    check(rc == 0 and out.count("stopped the ssh tunnel to") == 2, out + err)
    # --tunnel-stop only returns once everything has been cleaned up
    check(not any(alive(p) for p in pids), "ssh still running")
    check(env.socks() == [] and env.socks(".fwd") == [], "sockets left")
    wait_for(lambda: not env.daemon_pids(), timeout=2, what="daemon exit")
    check("exiting: stopped by request" in env.log_text(), env.log_text())
    rc, out, err = env.run(["--tunnel-bogus"], gce=False)
    check(rc == 2 and "usage: gce-ssh-curl" in err, err)


@test
def passthrough(env):
    """curl's exit status and output are passed through untouched"""
    rc, out, err = env.run(["--version"], gce=False,
                           env={"SSH_CURL_DIR": "/nonexistent/dir"})
    check(rc == 0 and out.startswith("curl "), out + err)
    check(env.ssh_pids() == [], "a tunnel was set up for --version")
    rc, out, err = env.get(path="/status", extra=["-f", "-X", "POST"])
    check(rc == 22 and out == "", "%d %r" % (rc, out))
    # certificate checking is still done by curl, end to end
    rc, out, err = env.run(["-s", "--cacert", env.cert_path,
                            "https://wrong.example.com/"])
    check(rc == 60, "rc=%d" % rc)
    rc, out, err = env.run(["-s", "https://%s/" % HOST])
    check(rc == 60, "rc=%d" % rc)
    rc, out, err = env.run(["-s", "-k", "-w", "%{http_code}", "-o",
                            "/dev/null", "https://%s/" % HOST])
    check(rc == 0 and out == "200", "%d %r" % (rc, out))
    # SSH_CURL_CURL
    mycurl = os.path.join(env.bin, "mycurl")
    env.script(mycurl, 'echo "mycurl: $*"\n')
    rc, out, err = env.run(["-s", "https://%s/" % HOST],
                           {"SSH_CURL_CURL": mycurl})
    check(rc == 0 and out.startswith("mycurl: --unix-socket %s/xfstests-ltm."
                                     % env.sockdir) and
          out.endswith(".sock -s https://%s/\n" % HOST), out)


@test
def config_errors(env):
    """Mistakes in the configuration are reported, with exit status 2"""
    url = "https://%s/" % HOST
    for e, text in [
            ({"SSH_CURL_SSH": "", "GCE_INSTANCE": ""},
             "GCE_INSTANCE is not set"),
            ({"SSH_CURL_SSH": "", "GCE_ZONE": ""}, "GCE_ZONE is not set"),
            ({"SSH_CURL_SSH": "", "GCE_PROJECT": ""},
             "GCE_PROJECT is not set"),
            ({"SSH_CURL_SSH": "ssh {nosuch}"}, "bad value for SSH_CURL_SSH"),
            ({"SSH_CURL_SSH": "ssh 'unbalanced"},
             "bad value for SSH_CURL_SSH"),
            ({"SSH_CURL_SSH": "ssh {instance}", "GCE_INSTANCE": ""},
             "SSH_CURL_SSH uses {instance}, but GCE_INSTANCE is not set"),
            ({"SSH_CURL_SSH": "ssh {project_dns}", "GCE_PROJECT": ""},
             "SSH_CURL_SSH uses {project_dns}, but GCE_PROJECT is not set"),
            ({"SSH_CURL_IDLE": "soon"}, "bad value for SSH_CURL_IDLE"),
            ({"SSH_CURL_TIMEOUT": "-1"}, "bad value for SSH_CURL_TIMEOUT"),
            ({"SSH_CURL_DIR": env.tmp + "/" + "x" * 90}, "is too long"),
    ]:
        rc, out, err = env.run(["-s", url], e)
        check(rc == 2 and text in err and out == "",
              "%r: %d %s" % (e, rc, err))
    # A directory which other users could get at is refused
    bad = os.path.join(env.tmp, "open")
    os.mkdir(bad)
    os.chmod(bad, 0o755)
    rc, out, err = env.run(["-s", url], {"SSH_CURL_DIR": bad})
    check(rc == 2 and "accessible to other users" in err, err)
    link = os.path.join(env.tmp, "link")
    os.symlink(env.tmp, link)
    rc, out, err = env.run(["-s", url], {"SSH_CURL_DIR": link})
    check(rc == 2 and "not a directory which belongs to you" in err, err)
    check(env.ssh_pids() == [], "ssh was started")
    # Domain-scoped project expansion in {project_dns}
    rc, out, err = env.get({"GCE_PROJECT": "google.com:my-proj"})
    check(rc == 0 and out == "index page", "%d %r %s" % (rc, out, err))
    check(env.ssh_argv()[-1][0] ==
          "root@xfstests-ltm.us-central1-c.my-proj.google.com",
          str(env.ssh_argv()))
    env.reset()
    # XDG_RUNTIME_DIR is used if SSH_CURL_DIR is not set
    xdg = os.path.join(env.tmp, "xdg")
    os.mkdir(xdg, 0o700)
    e = dict(env.env)
    del e["SSH_CURL_DIR"]
    e.update(env.gce)
    e["XDG_RUNTIME_DIR"] = xdg
    try:
        p = subprocess.run([SSH_CURL, "-sk", url], env=e,
                           stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                           stderr=subprocess.PIPE)
        check(p.returncode == 0 and p.stdout == b"index page",
              p.stderr.decode())
        xdg_dir = os.path.join(xdg, "ssh-curl")
        st = os.stat(xdg_dir)
        check(stat.S_IMODE(st.st_mode) == 0o700 and
              len([n for n in os.listdir(xdg_dir)
                   if n.endswith(".sock")]) == 1, "XDG_RUNTIME_DIR not used")
    finally:
        subprocess.run([SSH_CURL, "--tunnel-stop"], env=e,
                       stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL)


@test
def default_command(env):
    """Without SSH_CURL_SSH, gcloud compute ssh is used, as a process group"""
    # This gcloud does what the real one does: it runs ssh as a child
    # process (it does not exec it), with the arguments after "--"
    # placed after the destination.
    gcloud_log = os.path.join(env.tmp, "gcloud.argv")
    env.script(
        os.path.join(env.bin, "gcloud"),
        'printf "%%s\\n" "$*" >> %s\n'
        'while [ "$1" != "--" ]; do shift; done; shift\n'
        '%s %s root@1.2.3.4 "$@" &\n'
        'wait $!\n' % (
            shlex.quote(gcloud_log),
            shlex.quote(sys.executable),
            shlex.quote(os.path.join(HELPER_DIR, "fake_ssh.py"))))
    e = {"SSH_CURL_SSH": "", "PATH": env.bin + ":" + env.env["PATH"]}
    rc, out, err = env.get(e)
    check(rc == 0 and out == "index page", "%d %r %s" % (rc, out, err))
    argv = open(gcloud_log).read().strip()
    check(argv.startswith("--project test-proj compute ssh -q "
                          "root@xfstests-ltm --zone us-central1-c -- -T -N "
                          "-o BatchMode=yes ") and
          argv.endswith(":127.0.0.1:443"), argv)
    spid = env.ssh_pids()[0]
    fields = proc_stat_after_comm(spid)
    check(fields is not None, "ssh stat disappeared")
    gpid = int(fields[1])   # its parent
    check(alive(gpid) and gpid != env.daemon_pids()[0], "no gcloud process?")
    rc, out, err = env.run(["--tunnel-stop"], gce=False)
    check(not alive(spid) and not alive(gpid),
          "gcloud (%d) or ssh (%d) is still running" % (gpid, spid))
    rc, out, err = env.get(dict(e, GCE_ACCOUNT="me@example.com",
                                SSH_CURL_USER="testuser",
                                SSH_CURL_REMOTE="10.0.0.5:8443"))
    argv = open(gcloud_log).read().strip().split("\n")[-1]
    check(argv.startswith("--account me@example.com --project test-proj "
                          "compute ssh -q testuser@xfstests-ltm --zone "
                          "us-central1-c -- -T -N ") and
          argv.endswith(":10.0.0.5:8443"), argv)
    check(len(env.socks()) == 1, "sockets: %r" % env.socks())


@test
def real_ssh(env):
    """With a real ssh and sshd: the whole thing, end to end"""
    ssh = env.start_sshd()
    e = {"SSH_CURL_SSH": ssh,
         "SSH_CURL_REMOTE": "127.0.0.1:%d" % env.ltm_port}
    env.bash("true")
    rc, out0, err0 = env.bash(LTM + KCS, {"PATH": env.curlbin + ":" +
                                          env.env["PATH"]})
    base = strip(env.ltm_requests())
    env.reset()
    ssh = env.start_sshd()
    e = {"SSH_CURL_SSH": ssh,
         "SSH_CURL_REMOTE": "127.0.0.1:%d" % env.ltm_port}
    t0 = time.time()
    rc, out, err = env.bash("GCE_CURL=gce-ssh-curl\n" + LTM + KCS, e)
    check(rc == 0 and out == out0, "rc=%d\n%s\n%s" % (rc, out, err))
    check(strip(env.ltm_requests()) == base, "requests differ")
    check(err.count("setting up an ssh tunnel") == 2, err)
    print("    (2 tunnels and 5 requests in %.1f seconds)"
          % (time.time() - t0), end="")
    log = env.log_text()
    check("the tunnel is ready" in log, log)
    # ssh's forwarding socket is private, too
    for n in env.socks(".fwd"):
        st = os.stat(os.path.join(env.sockdir, n))
        check(stat.S_IMODE(st.st_mode) & 0o077 == 0, "fwd socket mode")

    def clear_tunnels():
        env.run(["--tunnel-stop"], gce=False)
        wait_for(lambda: not env.daemon_pids(), timeout=3, what="daemon exit")
        shutil.rmtree(env.sockdir, ignore_errors=True)

    # The server isn't listening: ssh says so in the log, curl fails,
    # and the tunnel stays up
    clear_tunnels()
    dead_port = unused_tcp_port()
    bad = dict(e, SSH_CURL_REMOTE="127.0.0.1:%d" % dead_port)
    rc, out, err = env.get(bad)
    check(rc != 0 and out == "", "%d %r %s" % (rc, out, err))
    time.sleep(0.3)
    check("open failed: connect failed: Connection refused"
          in env.log_text(), env.log_text())
    check(len(env.daemon_pids()) == 1, "the daemon exited")
    # The connection breaks (sshd's child for it is killed): the daemon
    # notices and exits, and the next use sets up a new tunnel
    clear_tunnels()
    rc, out, err = env.get(e)
    check(rc == 0 and out == "index page", "%d %r %s" % (rc, out, err))
    kids = child_pids(env.sshd.pid)
    check(kids, "no sshd child?")
    for k in kids:
        kill_tree(k)
    wait_for(lambda: not env.daemon_pids(), timeout=5, what="daemon exit")
    check("exiting: ssh exited (status 255)" in env.log_text(),
          env.log_text())
    rc, out, err = env.get(e)
    check(rc == 0 and out == "index page" and
          "setting up an ssh tunnel" in err, "%d %r %s" % (rc, out, err))
    clear_tunnels()
    # Authentication fails (wrong key): the reason is shown
    rc, out, err = env.get(dict(e, SSH_CURL_SSH=ssh.replace(
        "/userkey", "/hostkey")))
    check(rc == 7 and "Permission denied" in err and
          "could not set up an ssh tunnel" in err, "%d %s" % (rc, err))
    # sshd is not there at all
    env.sshd.kill()
    env.sshd.wait()
    rc, out, err = env.get(e)
    check(rc == 7 and "Connection refused" in err, "%d %s" % (rc, err))


def main():
    global verbose
    ap = argparse.ArgumentParser(
        description="Local end-to-end selftest for gce-ssh-curl "
        "(no real GCP access required).")
    ap.add_argument("-v", "--verbose", action="store_true",
                    help="show subprocess stdout/stderr on stderr")
    ap.add_argument("-k", default="", metavar="SUBSTRING",
                    help="run only tests whose name contains SUBSTRING")
    ap.add_argument("--list", action="store_true",
                    help="list available tests and exit")
    ap.add_argument("--keep", action="store_true",
                    help="preserve the temporary directory after running")
    ap.add_argument("--repo", default=None, metavar="PATH",
                    help="path to xfstests-bld root "
                    "(default: auto-detected relative to this script)")
    args = ap.parse_args()
    if args.list:
        for t in TESTS:
            print("%-20s %s" % (t.__name__, t.__doc__.strip()))
        return 0
    verbose = args.verbose

    resolve_repo(args.repo or DEFAULT_REPO)
    tools = require_tools()
    env = Env(tools)
    failed = []
    try:
        for t in TESTS:
            if args.k and args.k not in t.__name__:
                continue
            env.reset()
            sys.stdout.write("%-18s %s\n" % (t.__name__, t.__doc__.strip()))
            sys.stdout.flush()
            t0 = time.time()
            try:
                t(env)
                print("    PASS (%.1fs)" % (time.time() - t0))
            except Skip as e:
                print("    SKIP: %s" % e)
            except Failure as e:
                failed.append(t.__name__)
                print("    FAIL: %s" % e)
            except Exception:
                failed.append(t.__name__)
                print("    ERROR: %s" % traceback.format_exc())
    finally:
        env.cleanup(args.keep or bool(failed))
    print("\n%d failed: %s" % (len(failed), " ".join(failed)) if failed
          else "\nall passed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
