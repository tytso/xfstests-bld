#!/usr/bin/env python3
"""End-to-end tests for the GCE_CURL hook and the GCS relay.

Everything runs locally, against fakes:

  fake_ltm.py     stands in for the LTM/KCS server (HTTPS, CN-only cert)
  fake_gcp.py     stands in for the GCE metadata server and GCS
  fake_gcloud.py  stands in for the gcloud command (talks to fake_gcp.py)

What is real: the shell functions in run-fstests/util (run_gce_curl is
extracted from get-config; gce-ltm-funcs and gce-kcs-funcs are sourced
as they are), run-fstests/util/gce-gcs-curl, the appliance's
gce-gcs-relay daemon, curl, and jq.

Usage: test_gcs_relay.py [-v] [-k SUBSTRING] [--list] [--keep] [--repo PATH]
"""

import argparse
import base64
import fcntl
import json
import os
import re
import shlex
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import traceback
import urllib.parse
import urllib.request

HELPER_DIR = os.path.dirname(os.path.realpath(__file__))
DEFAULT_REPO = os.path.dirname(os.path.dirname(HELPER_DIR))
REPO = DEFAULT_REPO
RELAY = ""
GCS_CURL = ""
HOST = "a.test-proj.gce-xfstests"
PREFIX = "gcs-relay/xfstests-ltm-test-proj/"
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))

verbose = False


class Failure(Exception):
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
        time.sleep(0.1)
    raise Failure("timed out waiting for " + what)


def resolve_repo(repo_path):
    """Locate the xfstests-bld repository root and targets under test."""
    global REPO, RELAY, GCS_CURL
    REPO = os.path.realpath(repo_path)
    RELAY = os.path.join(
        REPO, "test-appliance", "files", "usr", "local", "lib",
        "gce-gcs-relay")
    GCS_CURL = os.path.join(REPO, "run-fstests", "util", "gce-gcs-curl")
    missing = [p for p in (
        RELAY, GCS_CURL,
        os.path.join(REPO, "run-fstests", "util", "get-config"),
        os.path.join(REPO, "run-fstests", "util", "gce-ltm-funcs"),
        os.path.join(REPO, "run-fstests", "util", "gce-kcs-funcs"),
    ) if not os.path.isfile(p)]
    if missing:
        raise SystemExit(
            "cannot find expected xfstests-bld files under %s:\n  %s"
            % (REPO, "\n  ".join(missing)))


def require_tools():
    """Return resolved paths for the external tools required by this suite."""
    tools = {}
    for name in ("bash", "curl", "jq", "openssl"):
        path = shutil.which(name)
        if not path:
            raise SystemExit("required tool not found in PATH: %s" % name)
        tools[name] = path
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


class Env:
    def __init__(self, tools):
        self.tools = tools
        self.tmp = tempfile.mkdtemp(prefix="gcsrelay.")
        self.procs = []
        self.nrelay = 0
        self.ntest = 0
        self.env = dict(os.environ)
        for k in list(self.env):
            if k.lower() in ("http_proxy", "https_proxy", "no_proxy",
                             "all_proxy"):
                del self.env[k]
        # Avoid writing __pycache__ inside the git working tree.
        self.env["PYTHONDONTWRITEBYTECODE"] = "1"
        self.bin = os.path.join(self.tmp, "bin")
        self.curlbin = os.path.join(self.tmp, "curlbin")
        self.rundir = os.path.join(self.tmp, "run-fstests")
        self.pkidir = os.path.join(self.tmp, "pki")
        for d in (self.bin, self.curlbin, self.rundir, self.pkidir):
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
        self.gcloud_log = os.path.join(self.tmp, "gcloud.log")
        self.gcloud_lock = os.path.join(self.tmp, "gcloud.lock")
        open(self.ltm_log, "w").close()
        open(self.gcloud_lock, "w").close()
        self.ltm_port = self.start_server(
            [sys.executable, os.path.join(HELPER_DIR, "fake_ltm.py"),
             "--cert", self.cert_path, "--key", self.key_path,
             "--log", self.ltm_log, "--slow", "30"], "ltm")
        self.gcp_port = self.start_server(
            [sys.executable, os.path.join(HELPER_DIR, "fake_gcp.py")], "gcp")
        self.gcp_url = "http://127.0.0.1:%d" % self.gcp_port

        self.script(
            os.path.join(self.bin, "gcloud"),
            'exec %s %s "$@"\n' % (
                shlex.quote(sys.executable),
                shlex.quote(os.path.join(HELPER_DIR, "fake_gcloud.py"))))
        # A curl wrapper which directs connections to the fake LTM server
        # (the real server listens on port 443).
        self.script(
            os.path.join(self.curlbin, "curl"),
            'exec %s --connect-to "::127.0.0.1:%d" "$@"\n'
            % (shlex.quote(tools["curl"]), self.ltm_port))
        self.env["PATH"] = self.bin + ":" + self.env["PATH"]
        self.env["FAKE_GCP_URL"] = self.gcp_url
        self.env["FAKE_GCLOUD_LOG"] = self.gcloud_log
        self.env["FAKE_GCLOUD_LOCK"] = self.gcloud_lock
        self.env["HOME"] = self.tmp

        for kind in ("ltm", "kcs"):
            with open(os.path.join(self.rundir,
                                   ".%s_instance_test-proj" % kind),
                      "w") as f:
                K = kind.upper()
                f.write("GCE_%s_SERVER_CERT=%s\n"
                        % (K, shlex.quote(self.cert_path)))
                f.write("GCE_%s_NAME=xfstests-%s\n" % (K, kind))
                f.write("GCE_%s_EXT_IP=127.0.0.1\n" % K)
                f.write("GCE_%s_PWD=sekrit-pw\n" % K)

        self.prelude = """
DIR=%(rundir)s
GCE_PROJECT=test-proj
GCE_ZONE=us-central1-c
GS_BUCKET=test-bucket
GCE_ACCOUNT=
eval "$(sed -n '/^    function run_gce_curl ()/,/^    }/p' \\
        %(get_config)s)"
function run_gcloud () { gcloud --project "$GCE_PROJECT" "$@"; }
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

    def start_relay(self, *extra, instance=None, md_instance=None,
                    project=None, md_project=None, name="relay", fast=True,
                    server_port=None):
        self.nrelay += 1
        log = os.path.join(self.tmp, "%s-%d.log" % (name, self.nrelay))
        md_inst = md_instance or instance
        md_proj = md_project or project
        if md_inst and md_proj:
            md_url = self.gcp_url + "/computeMetadata/v1.%s@%s" % (
                md_inst, md_proj)
        elif md_inst:
            md_url = self.gcp_url + "/computeMetadata/v1." + md_inst
        else:
            md_url = self.gcp_url + "/computeMetadata/v1"
        cmd = [sys.executable, RELAY,
               "--metadata-url", md_url,
               "--storage-url", self.gcp_url,
               "--server-url", "https://127.0.0.1:%d"
               % (server_port or self.ltm_port)]
        if fast:
            cmd += ["--burst-secs", "5", "--active-secs", "12",
                    "--active-interval", "3", "--response-ttl", "4"]
        if instance:
            cmd += ["--instance", instance]
        if project:
            cmd += ["--project", project]
        cmd += list(extra)
        f = open(log, "a")
        p = subprocess.Popen(cmd, env=self.env, stdout=f, stderr=f,
                             stdin=subprocess.DEVNULL)
        p.logpath = log
        self.procs.append(p)
        wait_for(lambda: "relaying requests" in open(log).read(),
                 what="relay to start")
        return p

    def stop(self, p, sig=signal.SIGTERM):
        if p.poll() is None:
            p.send_signal(sig)
            try:
                p.wait(10)
            except subprocess.TimeoutExpired:
                p.kill()
                p.wait()
        if p in self.procs:
            self.procs.remove(p)
        return p.returncode

    def bash(self, script, env=None, timeout=200, prelude=True):
        e = dict(self.env)
        e.update(env or {})
        p = subprocess.run([self.tools["bash"], "-c",
                            (self.prelude if prelude else "") + script],
                           env=e, stdin=subprocess.DEVNULL,
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                           timeout=timeout)
        out, err = p.stdout.decode(), p.stderr.decode()
        if verbose:
            sys.stderr.write("--- rc=%d\n--- stdout:\n%s--- stderr:\n%s"
                             % (p.returncode, out, err))
        return p.returncode, out, err

    def gcs_curl(self, args, env=None, instance="xfstests-ltm", timeout=200,
                 stdin=None):
        e = dict(self.env)
        e.update({"GS_BUCKET": "test-bucket", "GCE_PROJECT": "test-proj",
                  "GCE_ZONE": "us-central1-c", "GCE_INSTANCE": instance})
        e.update(env or {})
        p = subprocess.run([GCS_CURL] + args, env=e, input=stdin,
                           stdin=None if stdin is not None
                           else subprocess.DEVNULL,
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                           timeout=timeout)
        out, err = p.stdout, p.stderr.decode()
        if verbose:
            sys.stderr.write("--- rc=%d\n--- stdout:\n%r\n--- stderr:\n%s"
                             % (p.returncode, out, err))
        return p.returncode, out, err

    def gcp(self, method, path, data=None, token=True, **q):
        url = self.gcp_url + path
        if q:
            url += "?" + urllib.parse.urlencode(q)
        headers = {"X-Fake-Gcloud": "1"}
        if token:
            req = urllib.request.Request(
                self.gcp_url + "/computeMetadata/v1/instance/"
                "service-accounts/default/token",
                headers={"Metadata-Flavor": "Google", "X-Fake-Gcloud": "1"})
            tok = json.loads(OPENER.open(req).read().decode())
            headers["Authorization"] = "Bearer " + tok["access_token"]
        req = urllib.request.Request(url, data=data, headers=headers,
                                     method=method)
        try:
            with OPENER.open(req) as r:
                return r.status, r.read()
        except urllib.error.HTTPError as e:
            return e.code, e.read()

    def stats(self):
        return json.loads(self.gcp("GET", "/_test/stats", token=False)[1]
                          .decode())

    def objects(self):
        return self.stats()["objects"]

    def reset(self):
        """Forget everything between tests"""
        for kind in ("ltm", "kcs"):
            try:
                os.unlink("%s/.%s_cookie_test-proj" % (self.rundir, kind))
            except FileNotFoundError:
                pass
        # Wait for any background gcloud commands started by THIS run's
        # previous test (each holds a shared flock on self.gcloud_lock).
        def gcloud_idle():
            with open(self.gcloud_lock, "r+") as fh:
                try:
                    fcntl.flock(fh.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                except BlockingIOError:
                    return False
                fcntl.flock(fh.fileno(), fcntl.LOCK_UN)
                return True
        wait_for(gcloud_idle, timeout=30,
                 what="background gcloud commands to finish")
        open(self.ltm_log, "w").close()
        self.ntest += 1
        self.gcloud_log = os.path.join(
            self.tmp, "gcloud-%d.log" % self.ntest)
        open(self.gcloud_log, "w").close()
        self.env["FAKE_GCLOUD_LOG"] = self.gcloud_log
        _, rbody = self.gcp("POST", "/_test/reset", data=b"", token=False)
        epoch = json.loads(rbody.decode())["epoch"]
        self.env["FAKE_GCLOUD_EPOCH"] = str(epoch)

    def ltm_requests(self):
        return [json.loads(l) for l in open(self.ltm_log) if l.strip()]

    def gcloud_calls(self):
        return [json.loads(l)["argv"] for l in open(self.gcloud_log)
                if l.strip()]

    def put_request(self, rid, req, instance="xfstests-ltm",
                    project="test-proj"):
        data = req if isinstance(req, bytes) else json.dumps(req).encode()
        st, _ = self.gcp("POST", "/upload/storage/v1/b/test-bucket/o",
                         data=data, uploadType="media",
                         name="gcs-relay/%s-%s/req/%s.json"
                         % (instance, project, rid))
        check(st == 200, "upload failed")

    def get_response(self, rid, instance="xfstests-ltm", project="test-proj"):
        st, body = self.gcp("GET", "/storage/v1/b/test-bucket/o/" +
                            urllib.parse.quote(
                                "gcs-relay/%s-%s/rsp/%s.json"
                                % (instance, project, rid),
                                safe=""), alt="media")
        return json.loads(body.decode()) if st == 200 else None

    def poke(self, value=None):
        st, _ = self.gcp("POST", "/_test/metadata", data=b"", token=False,
                         key="gcs_relay", value=value or str(time.time()))
        check(st == 200, "poke failed")

    def cleanup(self, keep=False):
        for p in list(self.procs):
            self.stop(p, signal.SIGKILL)
        if keep:
            sys.stderr.write("kept " + self.tmp + "\n")
        else:
            shutil.rmtree(self.tmp, ignore_errors=True)


LTM_INFO = '. $DIR/util/gce-ltm-funcs\nLTM_INFO=yes send_to_ltm unused\n'
CMDLINE = base64.b64encode(b"ltm -c ext4/4k generic/001").decode()
LTM_RUN = ('. $DIR/util/gce-ltm-funcs\n'
           'BUCKET_SUBDIR=sub COMMIT=v6.1 send_to_ltm %s\n' % CMDLINE)
KCS_RUN = ('. $DIR/util/gce-kcs-funcs\n'
           'COMMIT=v6.1 GIT_REPO=https://example.com/linux.git '
           'send_to_kcs %s\n' % CMDLINE)


def strip(reqs):
    """The parts of the requests seen by the server which must not depend
    on how the request got there"""
    out = []
    for r in reqs:
        h = r["headers"]
        out.append({
            "method": r["method"], "path": r["path"], "body": r["body"],
            "host": h.get("host"), "cookie": h.get("cookie"),
            "content-type": h.get("content-type"),
            "accept": h.get("accept"),
        })
    return out


TESTS = []


def test(fn):
    TESTS.append(fn)
    return fn


@test
def curl_default(env):
    """GCE_CURL unset: plain curl, found via $PATH, exactly as before"""
    e = {"PATH": env.curlbin + ":" + env.env["PATH"]}
    rc, out, err = env.bash(LTM_INFO, e)
    check(rc == 0, "rc=%d" % rc)
    check('"msg": "login succeeded"' in out, "no login output: " + out)
    check('"sharders": []' in out, "no status output: " + out)
    reqs = env.ltm_requests()
    check([r["path"] for r in reqs] == ["/login", "/status"], str(reqs))
    check(reqs[0]["headers"]["user-agent"].startswith("curl/"),
          "not real curl?")
    check(env.gcloud_calls() == [], "gcloud was run: %r" % env.gcloud_calls())
    jar = open(env.rundir + "/.ltm_cookie_test-proj").read()
    check(HOST + "\tFALSE\t/\tFALSE\t" in jar, "cookie jar: " + jar)
    # Second time around there is no login
    rc, out, err = env.bash(LTM_RUN, e)
    check(rc == 0 and '"msg": "Launching tests"' in out, out)
    check([r["path"] for r in env.ltm_requests()] ==
          ["/login", "/status", "/gce-xfstests"], "unexpected requests")
    env.baseline_ltm = strip(env.ltm_requests())
    env.baseline_out = out

    env.reset()
    rc, out, err = env.bash(KCS_RUN, e)
    check(rc == 0 and '"msg": "Launching tests"' in out, out + err)
    env.baseline_kcs = strip(env.ltm_requests())
    env.baseline_kcs_out = out
    check([r["path"] for r in env.baseline_kcs] ==
          ["/login", "/gce-xfstests"], "unexpected requests")


@test
def curl_pathname(env):
    """GCE_CURL set to a pathname; env vars are passed to the program"""
    shim = env.tmp + "/my curl"         # note the space
    env.script(shim, 'env | grep -E "^(GCE_|GS_)" | sort > %s\n'
               'exec %s "$@"\n' % (
                   shlex.quote(os.path.join(env.tmp, "shim.env")),
                   shlex.quote(os.path.join(env.curlbin, "curl"))))
    rc, out, err = env.bash('GCE_CURL="%s"\nGCE_ACCOUNT=me@example.com\n'
                            % shim + LTM_INFO)
    check(rc == 0 and '"sharders": []' in out, out + err)
    got = open(env.tmp + "/shim.env").read().split("\n")
    for want in ("GCE_INSTANCE=xfstests-ltm", "GCE_PROJECT=test-proj",
                 "GCE_ZONE=us-central1-c", "GS_BUCKET=test-bucket",
                 "GCE_ACCOUNT=me@example.com"):
        check(want in got, "%s not in %r" % (want, got))
    # The variables must not leak into the caller's environment
    rc, out, err = env.bash('GCE_CURL="%s"\n' % shim + LTM_INFO +
                            'echo "exported=[$(env | grep -c GCE_INSTANCE)]"')
    check("exported=[0]" in out, "GCE_INSTANCE leaked: " + out)
    # ... and for the KCS the instance name is the KCS's
    env.reset()
    rc, out, err = env.bash('GCE_CURL="%s"\n' % shim + KCS_RUN)
    check(rc == 0, out + err)
    check("GCE_INSTANCE=xfstests-kcs" in open(env.tmp + "/shim.env").read(),
          "wrong instance for kcs")


@test
def relay_ltm(env):
    """GCE_CURL=gce-gcs-curl: LTM login, ltm-info, and a test request"""
    relay = env.start_relay(fast=False)
    t0 = time.time()
    rc, out, err = env.bash("GCE_CURL=gce-gcs-curl\n" + LTM_INFO)
    t1 = time.time()
    check(rc == 0, "rc=%d err=%s" % (rc, err))
    check(err == "", "unexpected stderr: " + err)
    check('"msg": "login succeeded"' in out, "no login output: " + out)
    check('"sharders": []' in out, "no status output: " + out)
    jar = open(env.rundir + "/.ltm_cookie_test-proj").read()
    check(HOST + "\tFALSE\t/\tFALSE\t" in jar, "cookie jar: " + jar)
    mode = os.stat(env.rundir + "/.ltm_cookie_test-proj").st_mode & 0o777
    check(mode == 0o600, "cookie jar mode %o" % mode)
    rc, out, err = env.bash("GCE_CURL=gce-gcs-curl\n" + LTM_RUN)
    check(rc == 0 and err == "", "rc=%d err=%s" % (rc, err))
    check(out == env.baseline_out,
          "output differs from curl's:\n%s\nvs\n%s" % (out, env.baseline_out))
    got = strip(env.ltm_requests())
    for g, w in zip(got, env.baseline_ltm):
        check(g == w, "server saw\n%r\nbut with curl it saw\n%r" % (g, w))
    check(len(got) == len(env.baseline_ltm), "wrong number of requests")
    # the bodyless POST has a content-length of zero
    check(env.ltm_requests()[1]["headers"].get("content-length") == "0",
          "bodyless POST: %r" % env.ltm_requests()[1]["headers"])
    # gcloud is run the same way as run_gcloud does
    calls = env.gcloud_calls()
    check(all(c[:2] == ["--project", "test-proj"] for c in calls),
          "gcloud calls: %r" % calls)
    check(any(c[2:6] == ["compute", "-q", "instances", "add-metadata"] and
              "xfstests-ltm" in c and "us-central1-c" in c for c in calls),
          "no add-metadata call")
    # Nothing is left behind in the bucket
    wait_for(lambda: env.objects() == [], what="bucket to be empty")
    log = open(relay.logpath).read()
    check("POST /login: HTTP status 200" in log, "relay log: " + log)
    check("sekrit" not in log and "MTcZ" not in log,
          "secrets in the relay's log!")
    print("    (3 relayed requests took %.1f seconds in all, with a fake "
          "gcloud which takes 0.3s to start)" % (time.time() - t0), end="")
    check(env.stop(relay) == 0, "relay exit status")
    check("exiting" in open(relay.logpath).read(), "no exit message")


@test
def relay_kcs(env):
    """GCE_CURL=gce-gcs-curl: KCS login and a build request"""
    relay = env.start_relay(instance="xfstests-kcs")
    rc, out, err = env.bash("GCE_CURL=gce-gcs-curl\n" + KCS_RUN)
    check(rc == 0 and err == "", "rc=%d err=%s" % (rc, err))
    check(out == env.baseline_kcs_out, "output differs from curl's: " + out)
    got = strip(env.ltm_requests())
    check(got == env.baseline_kcs,
          "server saw\n%r\nbut with curl it saw\n%r" % (got, env.baseline_kcs))
    check(any("gcs-relay/xfstests-kcs-test-proj/req/" in " ".join(c)
              for c in env.gcloud_calls()), "wrong object names")
    wait_for(lambda: env.objects() == [], what="bucket to be empty")
    env.stop(relay)


@test
def relay_account(env):
    """GCE_ACCOUNT is passed to gcloud"""
    relay = env.start_relay()
    rc, out, err = env.bash("GCE_CURL=gce-gcs-curl\n"
                            "GCE_ACCOUNT=me@example.com\n" + LTM_INFO)
    check(rc == 0 and '"sharders": []' in out, out + err)
    calls = env.gcloud_calls()
    check(calls and all(c[:4] == ["--account", "me@example.com", "--project",
                                  "test-proj"] for c in calls),
          "gcloud calls: %r" % calls)
    env.stop(relay)


@test
def wrong_password(env):
    """A failed login looks the same as it does with curl"""
    bad = ('. $DIR/util/gce-ltm-funcs\nGCE_LTM_PWD=wrong\n'
           'LTM_INFO=yes send_to_ltm unused\necho "rc=$?"\n')
    rc, out1, err1 = env.bash(bad, {"PATH": env.curlbin + ":" +
                                    env.env["PATH"]})
    jar1 = open(env.rundir + "/.ltm_cookie_test-proj").read()
    env.reset()
    relay = env.start_relay()
    rc, out2, err2 = env.bash("GCE_CURL=gce-gcs-curl\n" + bad)
    jar2 = open(env.rundir + "/.ltm_cookie_test-proj").read()
    check("Login failed." in out1 and "rc=1" in out1, out1)
    check(out1 == out2, "curl:\n%s\nrelay:\n%s" % (out1, out2))
    check(HOST not in jar1 and HOST not in jar2, "cookie stored?")
    env.stop(relay)


@test
def cookie_interop(env):
    """Cookie jars are interchangeable between curl and gce-gcs-curl"""
    relay = env.start_relay()
    curl_env = {"PATH": env.curlbin + ":" + env.env["PATH"]}
    # login via the relay, then use the jar with real curl
    rc, out, err = env.bash("GCE_CURL=gce-gcs-curl\n" + LTM_INFO)
    check(rc == 0 and "login succeeded" in out, out + err)
    rc, out, err = env.bash(LTM_INFO, curl_env)
    check('"sharders": []' in out and "login" not in out, out + err)
    check([r["path"] for r in env.ltm_requests()] ==
          ["/login", "/status", "/status"], "requests")
    # and the other way around
    env.reset()
    rc, out, err = env.bash(LTM_INFO, curl_env)
    check(rc == 0 and "login succeeded" in out, out + err)
    rc, out, err = env.bash("GCE_CURL=gce-gcs-curl\n" + LTM_INFO)
    check('"sharders": []' in out and "login" not in out, out + err)
    check([r["path"] for r in env.ltm_requests()] ==
          ["/login", "/status", "/status"], "requests")
    env.stop(relay)


@test
def no_relay(env):
    """No relay daemon (e.g., an old appliance): times out cleanly"""
    t0 = time.time()
    rc, out, err = env.bash(
        "GCE_CURL=gce-gcs-curl\nexport GCE_GCS_CURL_TIMEOUT=5\n"
        ". $DIR/util/gce-ltm-funcs\n"
        "ltm_post_json -c $DIR/.ltm_cookie_test-proj -d '{}' "
        "https://$LTM_HOSTNAME/login; echo \"rc=${PIPESTATUS[0]}\"\n"
        "run_gce_curl xfstests-ltm -s https://$LTM_HOSTNAME/; echo \"rc=$?\"")
    elapsed = time.time() - t0
    check("rc=28" in out, out + err)
    check(out.count("rc=28") == 1, out)
    check("timed out after 5 seconds" in err, err)
    check("gce-gcs-relay service" in err, err)
    check(10 <= elapsed < 20, "took %.1f seconds" % elapsed)
    check(not os.path.exists(env.rundir + "/.ltm_cookie_test-proj"),
          "cookie jar was created")
    check(env.ltm_requests() == [], "server was contacted?!")
    wait_for(lambda: env.objects() == [], what="request to be cleaned up")


@test
def stale_request(env):
    """A request which is older than its timeout is never executed"""
    req = {"version": 1, "method": "POST", "path": "/gce-xfstests",
           "headers": [["Cookie", "single-session=MTcZTOKENabcDEF=="]],
           "body": base64.b64encode(b'{"orig_cmdline":"x"}').decode(),
           "timeout": 5}
    env.put_request("20260101000000-1-old", req)
    env.put_request("20260101000000-2-new", req)
    env.gcp("POST", "/_test/age", data=b"", token=False,
            name=PREFIX + "req/20260101000000-1-old.json", seconds="60")
    env.poke()
    relay = env.start_relay()
    wait_for(lambda: env.get_response("20260101000000-2-new"),
             what="response to the fresh request")
    time.sleep(1.5)
    check(env.get_response("20260101000000-1-old") is None,
          "stale request got a response")
    check(len(env.ltm_requests()) == 1, "server saw %d requests"
          % len(env.ltm_requests()))
    check(PREFIX + "req/20260101000000-1-old.json" not in env.objects(),
          "stale request was not removed")
    # (The age is 60 seconds, give or take the one second resolution
    # of the Date header which the relay uses as the current time.)
    check(re.search(r"-old: ignored, since it is (59|6\d) seconds old",
                    open(relay.logpath).read()), open(relay.logpath).read())
    env.stop(relay)


@test
def at_most_once(env):
    """Two relays racing for the same requests: each is executed once"""
    env.gcp("POST", "/_test/set", data=b"", token=False, page_size="3")
    relays = [env.start_relay(name="relay%d" % i) for i in range(3)]
    n = 12
    req = {"version": 1, "method": "POST", "path": "/gce-xfstests",
           "headers": [["Cookie", "single-session=MTcZTOKENabcDEF=="]],
           "timeout": 60}
    for i in range(n):
        r = dict(req)
        r["body"] = base64.b64encode(
            json.dumps({"orig_cmdline": "req%02d" % i}).encode()).decode()
        env.put_request("20260101000000-%02d" % i, r)
    env.poke()
    wait_for(lambda: all(env.get_response("20260101000000-%02d" % i)
                         for i in range(n)), timeout=30,
             what="all of the responses")
    time.sleep(2)
    seen = sorted(json.loads(r["body"])["orig_cmdline"]
                  for r in env.ltm_requests())
    check(seen == ["req%02d" % i for i in range(n)],
          "server saw: %r" % seen)
    for i in range(n):
        rsp = env.get_response("20260101000000-%02d" % i)
        body = json.loads(base64.b64decode(rsp["body"]).decode())
        check(body["echo"] == "req%02d" % i, "mismatched response")
    counts = [open(r.logpath).read().count("HTTP status 200")
              for r in relays]
    check(sum(counts) == n, "relay logs: %r" % counts)
    print("    (the three relays handled %r requests)" % counts, end="")
    for r in relays:
        env.stop(r)


@test
def dormant(env):
    """An unused relay does not touch GCS; it wakes up when signalled"""
    relay = env.start_relay("--debug")
    time.sleep(4)
    st = env.stats()["stats"]
    check(not any(k.startswith("gcs:") for k in st),
          "GCS was accessed while dormant: %r" % st)
    check(st.get("md:instance/attributes/", 0) <= 2,
          "metadata server polled too often: %r" % st)
    t0 = time.time()
    env.poke()
    wait_for(lambda: env.stats()["stats"].get("gcs:list"), timeout=5,
             what="relay to wake up")
    check(time.time() - t0 < 2, "took %.1f seconds to wake up"
          % (time.time() - t0))
    # burst: about one listing a second for 5 seconds, then every 3
    # seconds, until 12 seconds after the wakeup; then nothing
    time.sleep(5.5)
    burst = env.stats()["stats"]["gcs:list"]
    check(4 <= burst <= 8, "listings during the burst: %d" % burst)
    time.sleep(10)
    active = env.stats()["stats"]["gcs:list"]
    check(burst + 2 <= active <= burst + 5,
          "listings: %d, then %d" % (burst, active))
    check("going dormant" in open(relay.logpath).read(), "not dormant?")
    time.sleep(4)
    check(env.stats()["stats"]["gcs:list"] == active, "still listing")
    # An unrelated metadata change does not wake it up
    env.gcp("POST", "/_test/metadata", data=b"", token=False,
            key="ltm_wait", value="12345")
    time.sleep(2)
    check(env.stats()["stats"]["gcs:list"] == active,
          "woken up by ltm_wait")
    env.stop(relay)


@test
def stale_response(env):
    """Responses which are not picked up are removed by the relay"""
    relay = env.start_relay()
    req = {"version": 1, "method": "GET", "path": "/", "headers": [],
           "timeout": 60}
    env.put_request("20260101000000-9-orphan", req)
    env.poke()
    wait_for(lambda: env.get_response("20260101000000-9-orphan"),
             what="response")
    rsp = env.get_response("20260101000000-9-orphan")
    check(base64.b64decode(rsp["body"]) == b"index page", "body: %r" % rsp)
    wait_for(lambda: env.objects() == [], timeout=20,
             what="response to be removed")
    env.stop(relay)


@test
def started_late(env):
    """A relay which starts after gcs_relay was set looks right away"""
    env.poke("something")
    req = {"version": 1, "method": "GET", "path": "/", "headers": [],
           "timeout": 60}
    env.put_request("20260101000000-7-early", req)
    relay = env.start_relay()
    wait_for(lambda: env.get_response("20260101000000-7-early"), timeout=5,
             what="response")
    env.stop(relay)


@test
def signal_fails(env):
    """add-metadata fails (e.g., wrong zone): fail early, with the reason"""
    relay = env.start_relay()
    # (a) the relay is dormant: no response can come
    t0 = time.time()
    rc, out, err = env.gcs_curl(["-s", "https://%s/" % HOST],
                                {"GCE_ZONE": "us-east1-b"})
    elapsed = time.time() - t0
    check(rc == 7, "rc=%d %s" % (rc, err))
    check("could not signal the instance xfstests-ltm in zone us-east1-b"
          in err and "was not found" in err, err)
    check(30 < elapsed < 45, "took %.1f seconds" % elapsed)
    n = sum(1 for c in env.gcloud_calls() if "add-metadata" in c)
    check(n == 3, "%d add-metadata attempts" % n)
    wait_for(lambda: env.objects() == [], what="request to be cleaned up")
    check(env.ltm_requests() == [], "request was executed?")
    # (b) the relay has been used recently, so it notices the request
    # anyway
    rc, out, err = env.gcs_curl(["-s", "https://%s/" % HOST])
    check(rc == 0 and out == b"index page", "%d %r %s" % (rc, out, err))
    time.sleep(6)       # the burst is over; now it polls every 3 seconds
    rc, out, err = env.gcs_curl(["-s", "https://%s/" % HOST],
                                {"FAKE_GCLOUD_FAIL": "add-metadata"})
    check(rc == 0 and out == b"index page", "%d %r %s" % (rc, out, err))
    env.stop(relay)


@test
def pipeline_not_held(env):
    """Background gcloud processes don't keep our stdout/stderr open"""
    relay = env.start_relay()
    env.poke()
    time.sleep(1)               # the relay is now awake
    # Removing the response, and signalling the instance, will still be
    # going on (in the background) long after the response has arrived.
    e = {"FAKE_GCLOUD_DELAY_RM": "8", "FAKE_GCLOUD_DELAY_ADD_METADATA": "8"}
    t0 = time.time()
    rc, out, err = env.bash(
        "export GS_BUCKET GCE_PROJECT GCE_ZONE GCE_INSTANCE=xfstests-ltm\n"
        "%s -s https://%s/ 2>&1 | cat; echo \" rc=${PIPESTATUS[0]}\"\n"
        % (shlex.quote(GCS_CURL), HOST), e)
    elapsed = time.time() - t0
    check("index page rc=0" in out, out + err)
    check(elapsed < 6, "pipeline took %.1f seconds" % elapsed)
    # ... and they do finish what they were doing
    wait_for(lambda: env.objects() == [], timeout=20,
             what="response to be removed")
    env.stop(relay)


@test
def server_down(env):
    """The LTM server isn't running: curl-style exit status 7"""
    dead_port = unused_tcp_port()
    relay = env.start_relay(server_port=dead_port)
    rc, out, err = env.gcs_curl(["-s", "-c", env.tmp + "/jar-down",
                                 "https://%s/" % HOST])
    check(rc == 7, "rc=%d %s" % (rc, err))
    check(("relay on xfstests-ltm: cannot connect to the server at "
           "https://127.0.0.1:%d" % dead_port) in err, err)
    check(out == b"", "output: %r" % out)
    check(not os.path.exists(env.tmp + "/jar-down"), "jar was written")
    env.stop(relay)


@test
def server_slow(env):
    """The server takes too long: exit status 28, one way or another"""
    relay = env.start_relay()
    rc, out, err = env.gcs_curl(["-s", "-X", "POST", "-b",
                                 "single-session=MTcZTOKENabcDEF==",
                                 "-m", "9", "https://%s/slow" % HOST])
    check(rc == 28, "rc=%d %s" % (rc, err))
    # whichever side noticed first, the relay must have written its
    # own verdict
    wait_for(lambda: "did not respond within" in
             json.dumps([env.get_response(n.split("/")[-1][:-5])
                         for n in env.objects() if "/rsp/" in n]) or
             "the server did not respond" in open(relay.logpath).read(),
             timeout=15, what="relay's timeout")
    env.stop(relay)


@test
def bad_requests(env):
    """Malformed requests are rejected, with an explanation"""
    relay = env.start_relay()
    good = {"version": 1, "method": "GET", "path": "/", "headers": [],
            "timeout": 60}
    cases = {
        "a-garbage": b"this is not json",
        "b-version": dict(good, version=2),
        "c-method": dict(good, method="GET /evil"),
        "d-path": dict(good, path="/a b"),
        "e-path2": dict(good, path="http://example.com/"),
        "f-header": dict(good, headers=[["X-Foo", "a\r\nX-Evil: 1"]]),
        "g-header2": dict(good, headers=[["Bad Name", "x"]]),
        "h-body": dict(good, body="!!!not base64!!!"),
        "i-timeout": dict(good, timeout="soon"),
        "j-list": [1, 2, 3],
    }
    for rid, req in cases.items():
        env.put_request("20260101000000-" + rid, req)
    env.put_request("not a valid name", good)
    env.put_request("20260101000000-z-good", good)
    env.poke()
    wait_for(lambda: env.get_response("20260101000000-z-good"),
             what="response to the good request")
    for rid in cases:
        rsp = env.get_response("20260101000000-" + rid)
        check(rsp and rsp.get("kind") == "request" and rsp.get("error"),
              "%s: %r" % (rid, rsp))
    check(len(env.ltm_requests()) == 1, "server saw %d requests"
          % len(env.ltm_requests()))
    # The client reports such an error as exit status 56
    env.stop(relay)


@test
def hop_by_hop(env):
    """Connection-level headers from the client are not passed along"""
    relay = env.start_relay()
    req = {"version": 1, "method": "POST", "path": "/login",
           "headers": [["Host", "h.example"], ["Content-Length", "9999"],
                       ["Transfer-Encoding", "chunked"],
                       ["Connection", "keep-alive"], ["X-Test", "yes"]],
           "body": base64.b64encode(b'{"password":"sekrit-pw"}').decode(),
           "timeout": 60}
    env.put_request("20260101000000-hbh", req)
    env.poke()
    rsp = wait_for(lambda: env.get_response("20260101000000-hbh"),
                   what="response")
    check(rsp.get("status") == 200, "response: %r" % rsp)
    h = env.ltm_requests()[0]["headers"]
    check(h.get("content-length") == "24" and h.get("x-test") == "yes" and
          h.get("host") == "h.example" and "transfer-encoding" not in h and
          h.get("connection") == "close", "server saw headers %r" % h)
    names = [n.lower() for n, _ in rsp["headers"]]
    check("set-cookie" in names and "content-type" in names and
          "content-length" not in names, "response headers: %r" % names)
    env.stop(relay)


@test
def gcs_trouble(env):
    """Expired tokens and transient GCS errors are survived"""
    relay = env.start_relay()
    rc, out, err = env.gcs_curl(["-s", "https://%s/" % HOST])
    check(rc == 0 and out == b"index page", "%d %r %s" % (rc, out, err))
    env.gcp("POST", "/_test/expire-token", data=b"", token=False)
    for op in ("list", "get", "delete", "put"):
        env.gcp("POST", "/_test/fail", data=b"", token=False, op=op,
                count="2", status="503")
    rc, out, err = env.gcs_curl(["-s", "https://%s/" % HOST])
    check(rc == 0 and out == b"index page", "%d %r %s" % (rc, out, err))
    check(env.stats()["stats"].get("gcs:401", 0) >= 1, "no 401 was seen")
    # The metadata server misbehaving does not make the relay spin
    env.gcp("POST", "/_test/set", data=b"", token=False, no_etag="1")
    time.sleep(13)           # let it go dormant
    env.gcp("POST", "/_test/reset-stats", data=b"", token=False)
    time.sleep(5)
    n = env.stats()["stats"].get("md:instance/attributes/", 0)
    check(n <= 7, "%d metadata requests in 5 seconds" % n)
    env.gcp("POST", "/_test/set", data=b"", token=False, no_etag="0")
    env.gcp("POST", "/_test/fail", data=b"", token=False, op="md",
            count="2", status="503")
    rc, out, err = env.gcs_curl(["-s", "https://%s/" % HOST])
    check(rc == 0 and out == b"index page", "%d %r %s" % (rc, out, err))
    check("Traceback" not in open(relay.logpath).read(),
          open(relay.logpath).read())
    # A "not modified" answer from the metadata server is not an error
    before = open(relay.logpath).read().count("will retry")
    check(before >= 2, "the injected failures were not noticed?")
    env.gcp("POST", "/_test/fail", data=b"", token=False, op="md",
            count="3", status="304")
    time.sleep(4)
    check(open(relay.logpath).read().count("will retry") == before,
          open(relay.logpath).read())
    rc, out, err = env.gcs_curl(["-s", "https://%s/" % HOST])
    check(rc == 0 and out == b"index page", "%d %r %s" % (rc, out, err))
    env.stop(relay)


@test
def sigterm_busy(env):
    """SIGTERM while a request is in flight: the request is finished"""
    relay = env.start_relay()
    req = {"version": 1, "method": "POST", "path": "/slow",
           "headers": [["Cookie", "single-session=MTcZTOKENabcDEF=="]],
           "timeout": 8}
    env.put_request("20260101000000-busy", req)
    env.poke()
    wait_for(lambda: env.ltm_requests(), what="request to reach the server")
    relay.send_signal(signal.SIGTERM)
    time.sleep(1)
    check(relay.poll() is None, "relay exited while busy")
    rsp = wait_for(lambda: env.get_response("20260101000000-busy"),
                   timeout=15, what="response")
    check(rsp.get("kind") == "timeout", "response: %r" % rsp)
    check(relay.wait(5) == 0, "exit status")
    env.procs.remove(relay)


@test
def vm_isolation(env):
    """Separate LTM and KCS relays share one bucket without cross-talk"""
    kcs_log = os.path.join(env.tmp, "kcs_srv.log")
    open(kcs_log, "w").close()
    kcs_port = env.start_server(
        [sys.executable, os.path.join(HELPER_DIR, "fake_ltm.py"),
         "--cert", env.cert_path, "--key", env.key_path,
         "--log", kcs_log], "kcs_srv")
    try:
        # Neither daemon is given --instance; each discovers its own name
        # (xfstests-ltm vs xfstests-kcs) from its VM metadata server.
        ltm_relay = env.start_relay("--debug", md_instance="xfstests-ltm",
                                    name="ltm_relay")
        kcs_relay = env.start_relay("--debug", md_instance="xfstests-kcs",
                                    name="kcs_relay", server_port=kcs_port)

        # Sending to LTM leaves the KCS relay completely dormant.
        rc, out, err = env.bash("GCE_CURL=gce-gcs-curl\n" + LTM_INFO)
        check(rc == 0 and '"sharders": []' in out and err == "", out + err)
        check("woken up" in open(ltm_relay.logpath).read(),
              open(ltm_relay.logpath).read())
        check("woken up" not in open(kcs_relay.logpath).read(),
              "KCS relay was woken by an LTM request:\n" +
              open(kcs_relay.logpath).read())
        check(open(kcs_log).read() == "", "KCS server received LTM traffic!")

        # Fire LTM and KCS commands simultaneously against the same bucket.
        ltm_cmd = base64.b64encode(b"for-ltm-only").decode()
        kcs_cmd = base64.b64encode(b"for-kcs-only").decode()
        p_ltm = subprocess.Popen(
            [env.tools["bash"], "-c", env.prelude + "GCE_CURL=gce-gcs-curl\n"
             ". $DIR/util/gce-ltm-funcs\nsend_to_ltm %s\n" % ltm_cmd],
            env=env.env, stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        p_kcs = subprocess.Popen(
            [env.tools["bash"], "-c", env.prelude + "GCE_CURL=gce-gcs-curl\n"
             ". $DIR/util/gce-kcs-funcs\nsend_to_kcs %s\n" % kcs_cmd],
            env=env.env, stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        o_ltm, e_ltm = p_ltm.communicate(timeout=60)
        o_kcs, e_kcs = p_kcs.communicate(timeout=60)
        check(p_ltm.returncode == 0 and e_ltm == b"", o_ltm.decode() + e_ltm.decode())
        check(p_kcs.returncode == 0 and e_kcs == b"", o_kcs.decode() + e_kcs.decode())
        check(ltm_cmd in o_ltm.decode() and kcs_cmd not in o_ltm.decode(),
              o_ltm.decode())
        check(kcs_cmd in o_kcs.decode() and ltm_cmd not in o_kcs.decode(),
              o_kcs.decode())

        ltm_reqs = env.ltm_requests()
        kcs_reqs = [json.loads(l) for l in open(kcs_log) if l.strip()]
        check([r["path"] for r in ltm_reqs] ==
              ["/login", "/status", "/gce-xfstests"], str(ltm_reqs))
        check(ltm_cmd in ltm_reqs[-1]["body"], str(ltm_reqs[-1]))
        check([r["path"] for r in kcs_reqs] ==
              ["/login", "/gce-xfstests"], str(kcs_reqs))
        check(kcs_cmd in kcs_reqs[-1]["body"], str(kcs_reqs[-1]))
        wait_for(lambda: env.objects() == [], what="bucket cleanup")
        env.stop(ltm_relay)
        env.stop(kcs_relay)
    finally:
        for p in list(env.procs):
            if p not in env.procs[:2]:
                env.stop(p, signal.SIGKILL)


@test
def project_isolation(env):
    """Two GCE projects with the same instance name safely share one bucket"""
    other_proj = "example.com:other-proj"
    other_log = os.path.join(env.tmp, "other_proj_srv.log")
    open(other_log, "w").close()
    other_port = env.start_server(
        [sys.executable, os.path.join(HELPER_DIR, "fake_ltm.py"),
         "--cert", env.cert_path, "--key", env.key_path,
         "--log", other_log], "other_proj_srv")
    try:
        # Both VMs are named xfstests-ltm and share test-bucket, but live in
        # different GCE projects (one with a domain-scoped project ID).
        r_a = env.start_relay("--debug", md_instance="xfstests-ltm",
                              md_project="test-proj", name="proj_a")
        r_b = env.start_relay("--debug", md_instance="xfstests-ltm",
                              md_project=other_proj, name="proj_b",
                              server_port=other_port)
        check("gcs-relay/xfstests-ltm-test-proj/" in open(r_a.logpath).read(),
              open(r_a.logpath).read())
        check(("gcs-relay/xfstests-ltm-%s/" % other_proj)
              in open(r_b.logpath).read(), open(r_b.logpath).read())

        # Sending to test-proj leaves example.com:other-proj dormant.
        rc, out, err = env.gcs_curl(
            ["-s", "-X", "POST", "-b", "single-session=MTcZTOKENabcDEF==",
             "-d", '{"orig_cmdline":"proj-a-only"}',
             "https://%s/gce-xfstests" % HOST])
        check(rc == 0 and err == "" and
              json.loads(out.decode())["echo"] == "proj-a-only",
              "%d %r %s" % (rc, out, err))
        check("woken up" not in open(r_b.logpath).read(),
              "other-proj relay was woken by test-proj!\n" +
              open(r_b.logpath).read())
        check(open(other_log).read() == "",
              "other-proj server saw test-proj traffic!")

        # Fire parallel requests to both projects' xfstests-ltm at once.
        e_a = dict(env.env, GS_BUCKET="test-bucket", GCE_PROJECT="test-proj",
                   GCE_ZONE="us-central1-c", GCE_INSTANCE="xfstests-ltm")
        e_b = dict(e_a, GCE_PROJECT=other_proj)
        p_a = subprocess.Popen(
            [GCS_CURL, "-s", "-X", "POST",
             "-b", "single-session=MTcZTOKENabcDEF==",
             "-d", '{"orig_cmdline":"from-a"}',
             "https://%s/gce-xfstests" % HOST],
            env=e_a, stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        p_b = subprocess.Popen(
            [GCS_CURL, "-s", "-X", "POST",
             "-b", "single-session=MTcZTOKENabcDEF==",
             "-d", '{"orig_cmdline":"from-b"}',
             "https://a.other-proj.gce-xfstests/gce-xfstests"],
            env=e_b, stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        o_a, err_a = p_a.communicate(timeout=60)
        o_b, err_b = p_b.communicate(timeout=60)
        check(p_a.returncode == 0 and err_a == b"" and
              json.loads(o_a.decode())["echo"] == "from-a",
              "%d %r %r" % (p_a.returncode, o_a, err_a))
        check(p_b.returncode == 0 and err_b == b"" and
              json.loads(o_b.decode())["echo"] == "from-b",
              "%d %r %r" % (p_b.returncode, o_b, err_b))
        other_reqs = [json.loads(l) for l in open(other_log) if l.strip()]
        check(len(other_reqs) == 1 and "from-b" in other_reqs[0]["body"],
              str(other_reqs))
        check(any(("gcs-relay/xfstests-ltm-%s/req/" % other_proj) in " ".join(c)
                  for c in env.gcloud_calls()), "wrong object path for proj_b")
        wait_for(lambda: env.objects() == [], what="bucket cleanup")
        env.stop(r_a)
        env.stop(r_b)
    finally:
        for p in list(env.procs):
            if p not in env.procs[:2]:
                env.stop(p, signal.SIGKILL)


@test
def parallel_requests(env):
    """Parallel gce-xfstests invocations execute concurrently without blocking"""
    relay = env.start_relay()
    e = dict(env.env)
    e.update({"GS_BUCKET": "test-bucket", "GCE_PROJECT": "test-proj",
              "GCE_ZONE": "us-central1-c", "GCE_INSTANCE": "xfstests-ltm",
              "FAKE_GCLOUD_DELAY": "0.1"})

    # (a) 4 slow requests (4s each on the server) complete in parallel,
    # well under the >=16s serial execution would take.
    t0 = time.time()
    procs = [
        subprocess.Popen(
            [GCS_CURL, "-s", "-X", "POST",
             "-b", "single-session=MTcZTOKENabcDEF==",
             "-d", json.dumps({"orig_cmdline": "slow-%d" % i}),
             "https://%s/delay/4" % HOST],
            env=e, stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        for i in range(4)
    ]
    outs = [p.communicate(timeout=30) for p in procs]
    elapsed = time.time() - t0
    for i, (p, (out, err)) in enumerate(zip(procs, outs)):
        check(p.returncode == 0 and err == b"",
              "slow-%d: rc=%d err=%r" % (i, p.returncode, err))
        check(json.loads(out.decode())["echo"] == "slow-%d" % i,
              "wrong echo for %d: %r" % (i, out))
    check(elapsed < 9.0, "4 x 4s requests serialized: took %.1fs" % elapsed)

    # (b) A fast request sent while a 6s request is in flight finishes
    # before the slow request completes (no head-of-line blocking).
    open(env.ltm_log, "w").close()
    slow = subprocess.Popen(
        [GCS_CURL, "-s", "-X", "POST",
         "-b", "single-session=MTcZTOKENabcDEF==",
         "-d", '{"orig_cmdline":"blocker"}',
         "https://%s/delay/6" % HOST],
        env=e, stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    wait_for(lambda: env.ltm_requests(), what="slow request to hit server")
    tf = time.time()
    fast_rc, fast_out, fast_err = env.gcs_curl(
        ["-s", "-X", "POST", "-b", "single-session=MTcZTOKENabcDEF==",
         "https://%s/status" % HOST], {"FAKE_GCLOUD_DELAY": "0.1"})
    fast_elapsed = time.time() - tf
    check(fast_rc == 0 and b'"sharders": []' in fast_out and fast_err == "",
          "%d %r %s" % (fast_rc, fast_out, fast_err))
    check(slow.poll() is None,
          "fast request waited until slow request finished (%.1fs)"
          % fast_elapsed)
    check(fast_elapsed < 3.5, "fast request took %.1fs" % fast_elapsed)
    s_out, s_err = slow.communicate(timeout=20)
    check(slow.returncode == 0 and json.loads(s_out.decode())["echo"] ==
          "blocker", "%d %r %r" % (slow.returncode, s_out, s_err))

    # (c) 4 parallel gce-xfstests windows (3 ltm runs + 1 ltm-info) starting
    # cold with no cookie jar yet.
    try:
        os.unlink(env.rundir + "/.ltm_cookie_test-proj")
    except FileNotFoundError:
        pass
    be = dict(env.env, FAKE_GCLOUD_DELAY="0.1")
    windows = []
    for i in range(3):
        tag = base64.b64encode(("win-%d" % i).encode()).decode()
        proc = subprocess.Popen(
            [env.tools["bash"], "-c", env.prelude + "GCE_CURL=gce-gcs-curl\n"
             ". $DIR/util/gce-ltm-funcs\nsend_to_ltm %s\n" % tag],
            env=be, stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        windows.append((tag, proc))
    info_proc = subprocess.Popen(
        [env.tools["bash"], "-c", env.prelude + "GCE_CURL=gce-gcs-curl\n" + LTM_INFO],
        env=be, stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    for tag, proc in windows:
        out, err = proc.communicate(timeout=60)
        check(proc.returncode == 0 and err == b"",
              "%s: rc=%d out=%s err=%s" % (tag, proc.returncode,
                                           out.decode(), err.decode()))
        check(('"echo": "%s"' % tag) in out.decode(),
              "expected %s in %s" % (tag, out.decode()))
    i_out, i_err = info_proc.communicate(timeout=60)
    check(info_proc.returncode == 0 and i_err == b"" and
          '"sharders": []' in i_out.decode(), i_out.decode() + i_err.decode())
    wait_for(lambda: env.objects() == [], what="bucket cleanup")
    print("    (4 x 4s parallel took %.1fs; fast-over-slow took %.1fs)"
          % (elapsed, fast_elapsed), end="")
    env.stop(relay)


@test
def metadata_conflict(env):
    """Concurrent add-metadata HTTP 412 fingerprint conflicts are handled"""
    # (a) 6 parallel clients wake a dormant relay while GCE rejects
    # overlapping setMetadata calls with HTTP 412 fingerprint conflicts.
    env.gcp("POST", "/_test/set", data=b"", token=False,
            md_conflict="1", md_delay="0.25")
    relay = env.start_relay()
    e = dict(env.env)
    e.update({"GS_BUCKET": "test-bucket", "GCE_PROJECT": "test-proj",
              "GCE_ZONE": "us-central1-c", "GCE_INSTANCE": "xfstests-ltm",
              "FAKE_GCLOUD_DELAY": "0.05"})
    procs = [
        subprocess.Popen(
            [GCS_CURL, "-s", "-X", "POST",
             "-b", "single-session=MTcZTOKENabcDEF==",
             "-d", json.dumps({"orig_cmdline": "mc-%d" % i}),
             "https://%s/gce-xfstests" % HOST],
            env=e, stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        for i in range(6)
    ]
    for i, p in enumerate(procs):
        out, err = p.communicate(timeout=60)
        check(p.returncode == 0 and err == b"",
              "mc-%d: rc=%d err=%r" % (i, p.returncode, err))
        check(json.loads(out.decode())["echo"] == "mc-%d" % i, repr(out))
    conflicts = env.stats()["stats"].get("md:412", 0)
    check(conflicts >= 3, "expected 412 conflicts, saw %d" % conflicts)
    env.stop(relay)

    # (b) A single client wakes a dormant relay even when its first
    # add-metadata collides (412) with an unrelated ltm_wait update.
    env.reset()
    relay = env.start_relay()
    # An unrelated ltm_wait update does not wake the relay, and the
    # client's first two add-metadata attempts fail with HTTP 412; its
    # jittered retry lands, wakes the relay, and delivers the request.
    st, _ = env.gcp("POST", "/_test/metadata", data=b"", token=False,
                    key="ltm_wait", value="999")
    check(st == 200, "ltm_wait set failed: %d" % st)
    env.gcp("POST", "/_test/fail", data=b"", token=False,
            op="md-set", count="2", status="412")
    rc, out, err = env.gcs_curl(["-s", "https://%s/" % HOST],
                                {"FAKE_GCLOUD_DELAY": "0.05"})
    check(rc == 0 and out == b"index page" and err == "",
          "%d %r %r" % (rc, out, err))
    check(env.stats()["stats"].get("md:412", 0) == 2,
          "expected 2 412s: %r" % env.stats()["stats"])
    env.stop(relay)

    # (c) A request taking > SIGNAL_GRACE (30s) still succeeds when its
    # own add-metadata fails every time, because a parallel request woke
    # the relay and the relay already claimed (deleted) our req object.
    env.reset()
    relay = env.start_relay()
    env.poke("sibling-wakeup")
    rc, out, err = env.gcs_curl(
        ["-s", "-X", "POST", "-b", "single-session=MTcZTOKENabcDEF==",
         "-d", '{"orig_cmdline":"long-claimed"}',
         "https://%s/delay/33" % HOST],
        {"FAKE_GCLOUD_FAIL": "add-metadata", "FAKE_GCLOUD_DELAY": "0.1"})
    check(rc == 0 and err == "" and
          json.loads(out.decode())["echo"] == "long-claimed",
          "rc=%d out=%r err=%s" % (rc, out, err))
    wait_for(lambda: env.objects() == [], what="bucket cleanup")
    env.stop(relay)


@test
def sigterm_parallel(env):
    """SIGTERM finishes already-claimed parallel requests, leaves unclaimed"""
    relay = env.start_relay("--max-workers", "2")
    for i in range(4):
        env.put_request("20260101000000-sp%d" % i, {
            "version": 1, "method": "POST", "path": "/delay/2",
            "headers": [["Cookie", "single-session=MTcZTOKENabcDEF=="]],
            "body": base64.b64encode(
                json.dumps({"orig_cmdline": "sp%d" % i}).encode()).decode(),
            "timeout": 30})
    env.poke()
    wait_for(lambda: len(env.ltm_requests()) == 2,
             what="2 requests to reach the server")
    relay.send_signal(signal.SIGTERM)
    check(relay.wait(10) == 0, "relay exit status")
    env.procs.remove(relay)
    # The two claimed requests got responses; the two unclaimed stay in req/
    for i in (0, 1):
        rsp = env.get_response("20260101000000-sp%d" % i)
        check(rsp and rsp.get("status") == 200, "sp%d: %r" % (i, rsp))
    remaining = [n for n in env.objects() if "/req/" in n]
    check(remaining == [PREFIX + "req/20260101000000-sp2.json",
                        PREFIX + "req/20260101000000-sp3.json"],
          "remaining requests: %r" % remaining)
    # A restarted relay picks up the remaining two cleanly.
    relay2 = env.start_relay()
    wait_for(lambda: (env.get_response("20260101000000-sp2") and
                      env.get_response("20260101000000-sp3")),
             what="remaining responses")
    check(len(env.ltm_requests()) == 4,
          "expected 4 total requests, saw %d" % len(env.ltm_requests()))
    env.stop(relay2)


@test
def once(env):
    """--once handles what is there and exits"""
    good = {"version": 1, "method": "GET", "path": "/", "headers": [],
            "timeout": 60}
    env.put_request("20260101000000-once", good)
    p = subprocess.run(
        [sys.executable, RELAY, "--once",
         "--metadata-url", env.gcp_url + "/computeMetadata/v1",
         "--storage-url", env.gcp_url,
         "--server-url", "https://127.0.0.1:%d" % env.ltm_port],
        env=env.env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        timeout=30)
    check(p.returncode == 0, p.stdout.decode())
    check(env.get_response("20260101000000-once"), "no response")


@test
def cli_options(env):
    """gce-gcs-curl's command-line handling"""
    relay = env.start_relay()
    url = "https://%s" % HOST
    jar = env.tmp + "/cli.jar"

    def run(args, **kw):
        return env.gcs_curl(args, **kw)

    # usage errors do not need (or touch) the relay
    env.reset()
    for args, code, text in [
            (["--bogus", url], 2, "option --bogus: is unknown"),
            (["-sZ", url], 2, "option -Z: is unknown"),
            (["-d"], 2, "option -d: requires parameter"),
            (["--data"], 2, "option --data: requires parameter"),
            (["-s"], 2, "no URL specified"),
            ([url, url], 2, "only one URL"),
            (["http://%s/" % HOST], 1, "only https://"),
            (["https://%s:8443/" % HOST], 7, "port 443"),
            (["-m", "soon", url], 2, "bad value for --max-time"),
            (["-d", "@" + os.path.join(env.tmp, "no-such-file"), url], 26,
             "cannot read"),
    ]:
        rc, out, err = run(args)
        check(rc == code and text in err and out == b"",
              "%r: rc=%d err=%s" % (args, rc, err))
    for var in ("GS_BUCKET", "GCE_PROJECT", "GCE_ZONE", "GCE_INSTANCE"):
        rc, out, err = run([url], env={var: ""})
        check(rc == 2 and var + " is not set" in err, "%s: %s" % (var, err))
    check(env.gcloud_calls() == [], "gcloud was run for a usage error")
    rc, out, err = run(["--help"])
    check(rc == 0 and b"Usage: gce-gcs-curl" in out, "help")
    rc, out, err = run(["-V"])
    check(rc == 0 and out.startswith(b"gce-gcs-curl "), "version")

    # clustered short options, attached parameters, --opt=value
    rc, out, err = run(["-sSk", "-XPOST", "--cookie-jar=" + jar,
                        "-d{\"password\":\"sekrit-pw\"}",
                        "--header=Content-Type: application/json",
                        "--resolve", HOST + ":443:10.0.0.1",
                        "--cacert", os.path.join(env.tmp, "ignored-cert.pem"), "--connect-timeout", "5",
                        url + "/login"])
    check(rc == 0 and b"login succeeded" in out, "%d %r %s" % (rc, out, err))
    r = env.ltm_requests()[-1]
    check(r["method"] == "POST" and r["body"] == '{"password":"sekrit-pw"}'
          and r["headers"]["content-type"] == "application/json", str(r))

    # -d implies POST and a default content type; several -d are joined
    rc, out, err = run(["-s", "-d", "a=1", "--data-raw", "@b=2", "-b", jar,
                        url + "/status"])
    r = env.ltm_requests()[-1]
    check(r["method"] == "POST" and r["body"] == "a=1&@b=2" and
          r["headers"]["content-type"] ==
          "application/x-www-form-urlencoded" and
          r["headers"]["cookie"] == "single-session=MTcZTOKENabcDEF==",
          str(r))

    # -d @file strips newlines, --data-binary @file doesn't; @- is stdin
    datafile = env.tmp + "/data"
    with open(datafile, "wb") as f:
        f.write(b'{"orig_cmdline":\r\n"xyz"}\n')
    rc, out, err = run(["-s", "-b", jar, "-d", "@" + datafile,
                        url + "/gce-xfstests"])
    check(env.ltm_requests()[-1]["body"] == '{"orig_cmdline":"xyz"}',
          env.ltm_requests()[-1]["body"])
    check(json.loads(out.decode())["echo"] == "xyz", out)
    rc, out, err = run(["-s", "-b", jar, "--data-binary", "@" + datafile,
                        url + "/gce-xfstests"])
    check(env.ltm_requests()[-1]["body"] == '{"orig_cmdline":\r\n"xyz"}\n',
          repr(env.ltm_requests()[-1]["body"]))
    rc, out, err = run(["-s", "-b", jar, "--json", "@-",
                        url + "/gce-xfstests"],
                       stdin=b'{"orig_cmdline": "from stdin"}')
    r = env.ltm_requests()[-1]
    check(r["body"] == '{"orig_cmdline": "from stdin"}' and
          r["headers"]["content-type"] == "application/json" and
          r["headers"]["accept"] == "application/json", str(r))

    # no data: GET; headers can be added, replaced, and removed
    rc, out, err = run(["-s", "-H", "X-One: 1", "-H", "Accept:", "-H",
                        "X-Empty;", "-A", "my-agent", "-e", "http://ref/",
                        "-H", "Host: other.example", url + "/?q=1"])
    r = env.ltm_requests()[-1]
    h = r["headers"]
    check(r["method"] == "GET" and r["path"] == "/?q=1" and
          h.get("x-one") == "1" and "accept" not in h and
          h.get("x-empty") == "" and h.get("user-agent") == "my-agent" and
          h.get("referer") == "http://ref/" and
          h.get("host") == "other.example", str(r))
    check(out == b"index page", repr(out))

    # -b with a literal cookie
    rc, out, err = run(["-s", "-X", "POST", "-b",
                        "single-session=MTcZTOKENabcDEF==; other=1",
                        url + "/status"])
    check(rc == 0 and b"sharders" in out, "%d %r" % (rc, out))

    # -i, -o, -f
    rc, out, err = run(["-s", "-i", url + "/"])
    check(out.startswith(b"HTTP/1.1 200 OK\r\n") and
          b"\r\nContent-Type: text/plain\r\n" in out and
          out.endswith(b"\r\n\r\nindex page"), repr(out))
    outfile = env.tmp + "/out"
    rc, out, err = run(["-s", "-o", outfile, url + "/"])
    check(rc == 0 and out == b"" and
          open(outfile, "rb").read() == b"index page", "-o")
    rc, out, err = run(["-s", "-X", "POST", url + "/status"])
    check(rc == 0 and out == b"Login failed\n" and err == "",
          "%d %r %s" % (rc, out, err))
    rc, out, err = run(["-s", "-f", "-X", "POST", url + "/status"])
    check(rc == 22 and out == b"" and err == "", "%d %r %s" % (rc, out, err))
    rc, out, err = run(["-f", "-X", "POST", url + "/status"])
    check(rc == 22 and "(22) The requested URL returned error: 403" in err,
          "%d %r %s" % (rc, out, err))

    # -v says what is going on, without giving away secrets
    rc, out, err = run(["-v", "-X", "POST", "-b", jar, url + "/status"])
    check(rc == 0 and "* running: gcloud" in err and "MTcZ" not in err,
          err)

    # cookie jar details: expired/deleted cookies, paths, domains
    with open(jar, "w") as f:
        f.write("# Netscape HTTP Cookie File\n\n"
                "#HttpOnly_.gce-xfstests\tTRUE\t/\tTRUE\t0\tsess\tS1\n"
                "%s\tFALSE\t/gce\tFALSE\t0\tdeep\tD1\n"
                "%s\tFALSE\t/\tFALSE\t1\told\tO1\n"
                "other.example\tFALSE\t/\tFALSE\t0\tnope\tN1\n"
                "malformed line\n" % (HOST, HOST))
    rc, out, err = run(["-s", "-b", jar, "-c", jar, url + "/gce-xfstests"])
    r = env.ltm_requests()[-1]
    check(r["headers"].get("cookie") == "sess=S1", str(r["headers"]))
    rc, out, err = run(["-s", "-b", jar, url + "/gce/x"])
    r = env.ltm_requests()[-1]
    check(r["headers"].get("cookie") == "deep=D1; sess=S1",
          str(r["headers"]))
    saved = open(jar).read()
    check("#HttpOnly_.gce-xfstests\tTRUE\t/\tTRUE\t0\tsess\tS1\n" in saved
          and "\told\t" not in saved and "\tnope\tN1\n" in saved, saved)
    env.stop(relay)


def main():
    global verbose
    ap = argparse.ArgumentParser(
        description="Local end-to-end selftest for gce-gcs-curl and "
        "gce-gcs-relay (no real GCP/GCS access required).")
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
            if args.k and args.k not in t.__name__ and \
               t.__name__ != "curl_default":
                continue
            env.reset()
            sys.stdout.write("%-18s %s\n" % (t.__name__, t.__doc__.strip()))
            sys.stdout.flush()
            t0 = time.time()
            try:
                t(env)
                print("    PASS (%.1fs)" % (time.time() - t0))
            except Failure as e:
                failed.append(t.__name__)
                print("    FAIL: %s" % e)
            except Exception:
                failed.append(t.__name__)
                print("    ERROR: %s" % traceback.format_exc())
            # no test may leave a relay running
            for p in env.procs[2:]:
                env.stop(p, signal.SIGKILL)
    finally:
        env.cleanup(args.keep or bool(failed))
    print("\n%d failed: %s" % (len(failed), " ".join(failed)) if failed
          else "\nall passed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
