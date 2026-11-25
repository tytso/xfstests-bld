#!/usr/bin/env python3
"""Offline end-to-end self-test suite for shared GCS bucket multi-project support.

Exercises client scripts (run-fstests/gce-xfstests, util/get-config,
util/gce-do-setup, util/gce-setup-cert, util/gce-launch-ltm,
util/gce-launch-kcs) and appliance scripts (test-appliance/files/usr/local/lib/
gce-funcs, gce-fetch-gs-files, gce-setup, gce-run-batch, gce-ltm.boot,
gce-ltm-batch-watcher) against a local fake gcloud (modeling both Flat and
Hierarchical Namespace GCS buckets) and a local HTTP GCE Metadata Server.

For backwards/forwards compatibility gating checks, the "old" client and "old"
appliance scripts are extracted directly from 'origin/master' in git.

Usage: test_shared_bucket.py [-v] [-k SUBSTRING] [--list] [--keep] [--repo PATH]
"""

import argparse
import base64
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import traceback

HELPER_DIR = os.path.dirname(os.path.realpath(__file__))
DEFAULT_REPO = os.path.dirname(os.path.dirname(HELPER_DIR))
REPO = DEFAULT_REPO
FAKE_GCLOUD = os.path.join(HELPER_DIR, "fake_gcloud.py")

verbose = False
TESTS = []


class Failure(Exception):
    pass


def test(fn):
    TESTS.append(fn)
    return fn


def check(cond, msg):
    if not cond:
        raise Failure(msg)


def resolve_repo(repo_path):
    global REPO
    REPO = os.path.realpath(repo_path)
    required = [
        os.path.join(REPO, "run-fstests", "gce-xfstests"),
        os.path.join(REPO, "run-fstests", "util", "get-config"),
        os.path.join(REPO, "run-fstests", "util", "gce-do-setup"),
        os.path.join(REPO, "run-fstests", "util", "gce-setup-cert"),
        os.path.join(REPO, "test-appliance", "files", "usr", "local", "lib",
                     "gce-funcs"),
        os.path.join(REPO, "test-appliance", "files", "usr", "local", "lib",
                     "gce-fetch-gs-files"),
    ]
    missing = [p for p in required if not os.path.isfile(p)]
    if missing:
        raise SystemExit("missing expected files under %s:\n  %s"
                         % (REPO, "\n  ".join(missing)))


def require_tools():
    tools = {}
    for name in ("bash", "curl", "jq", "openssl", "git"):
        path = shutil.which(name)
        if not path:
            raise SystemExit("required tool not found in PATH: %s" % name)
        tools[name] = path
    return tools


class MDSServer:
    """In-process HTTP GCE Metadata Server supporting per-VM context."""

    def __init__(self):
        self.lock = threading.Lock()
        self.vms = {}

        outer = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, fmt, *args):
                return

            def do_GET(self):
                if self.headers.get("Metadata-Flavor") != "Google":
                    self.send_response(403)
                    self.end_headers()
                    return
                # URL path: /computeMetadata/v1.<vm_key>/<rest>
                path = self.path.split("?", 1)[0]
                m = re.match(r"^/computeMetadata/v1\.([^/]+)/(.*)$", path)
                if not m:
                    self.send_response(404)
                    self.end_headers()
                    return
                vm_key, rel = m.group(1), m.group(2)
                with outer.lock:
                    vm = dict(outer.vms.get(vm_key, {}))
                if not vm:
                    self.send_response(404)
                    self.end_headers()
                    return

                if rel == "project/project-id":
                    if vm.get("fail_project_id"):
                        self.send_response(500)
                        self.end_headers()
                        return
                    val = vm.get("project_id", "")
                elif rel == "instance/zone":
                    val = "projects/100001/zones/" + vm.get(
                        "zone", "us-central1-c")
                elif rel == "instance/id":
                    val = str(vm.get("id", "424242"))
                elif rel == "instance/name":
                    val = vm.get("name", "xfstests-vm")
                elif rel.startswith("instance/attributes/"):
                    attr = rel[len("instance/attributes/"):]
                    attrs = vm.get("attributes", {})
                    if attr not in attrs:
                        self.send_response(404)
                        self.end_headers()
                        return
                    val = str(attrs[attr])
                else:
                    self.send_response(404)
                    self.end_headers()
                    return

                body = val.encode("utf-8")
                self.send_response(200)
                self.send_header("Metadata-Flavor", "Google")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.port = self.httpd.server_address[1]
        self.thread = threading.Thread(target=self.httpd.serve_forever,
                                       daemon=True)
        self.thread.start()

    def register_vm(self, vm_key, project_id, name="xfstests-test",
                    zone="us-central1-c", attributes=None,
                    fail_project_id=False):
        with self.lock:
            self.vms[vm_key] = {
                "project_id": project_id,
                "name": name,
                "zone": zone,
                "attributes": dict(attributes or {}),
                "fail_project_id": fail_project_id,
            }
        return "http://127.0.0.1:%d/computeMetadata/v1.%s" % (
            self.port, vm_key)

    def close(self):
        self.httpd.shutdown()
        self.httpd.server_close()


class OriginArchiveCache:
    """Extracts 'origin/master' trees once per test suite run."""

    def __init__(self, git_bin, base_tmp):
        self.root = os.path.join(base_tmp, "origin_master")
        os.makedirs(self.root, exist_ok=True)
        proc = subprocess.run(
            [git_bin, "-C", REPO, "archive", "origin/master",
             "run-fstests", "kernel-build", "test-appliance/files"],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        if proc.returncode != 0:
            raise SystemExit(
                "git archive origin/master failed:\n%s"
                % proc.stderr.decode("utf-8", "replace"))
        with tarfile.open(fileobj=io.BytesIO(proc.stdout), mode="r:") as tf:
            try:
                tf.extractall(self.root, filter="data")
            except TypeError:
                tf.extractall(self.root)


class Harness:
    def __init__(self, tools, origin_cache):
        self.tools = tools
        self.origin = origin_cache
        self.tmp = tempfile.mkdtemp(prefix="sharedbkt.")
        self.cleanup_Host_tmp = []
        self.mds = MDSServer()

        self.state_path = os.path.join(self.tmp, "gcp_state.json")
        self.lock_path = os.path.join(self.tmp, "gcp.lock")
        self.gcloud_log = os.path.join(self.tmp, "gcloud.log")
        open(self.lock_path, "w").close()
        open(self.gcloud_log, "w").close()

        self.bin = os.path.join(self.tmp, "bin")
        os.makedirs(self.bin, exist_ok=True)
        self._write_exec(
            os.path.join(self.bin, "gcloud"),
            '#!/bin/sh\nexec %s %s "$@"\n' % (
                shlex.quote(sys.executable), shlex.quote(FAKE_GCLOUD))
        )
        # Provide pwgen shim for gce-do-setup in minimal sandboxes
        self._write_exec(
            os.path.join(self.bin, "pwgen"),
            '#!/bin/sh\n%s rand -hex 8 | cut -c1-15\n'
            % shlex.quote(tools["openssl"])
        )
        # Stub dig and gsutil (used by origin/master gce-do-setup)
        self._write_exec(os.path.join(self.bin, "dig"), "#!/bin/sh\nexit 0\n")
        self._write_exec(
            os.path.join(self.bin, "gsutil"),
            '#!/bin/sh\nif [ "$1" = "acl" ] && [ "$2" = "get" ]; then exit 1; fi\nexit 0\n'
        )

        self.base_env = dict(os.environ)
        for k in list(self.base_env):
            if k.lower() in ("http_proxy", "https_proxy", "no_proxy",
                             "all_proxy") or k.startswith("GS_"):
                del self.base_env[k]
        self.base_env["PYTHONDONTWRITEBYTECODE"] = "1"
        self.base_env["FAKE_GCP_STATE"] = self.state_path
        self.base_env["FAKE_GCP_LOCK"] = self.lock_path
        self.base_env["FAKE_GCLOUD_LOG"] = self.gcloud_log
        self.base_env["GCE_FETCH_RETRY_DELAY"] = "0"
        self.base_env["MDS_TRIES"] = "0"
        self.base_env["PATH"] = self.bin + ":" + self.base_env.get("PATH", "")

        self.state = {
            "projects": {
                "proj-a": {"number": "100001"},
                "proj-b": {"number": "100002"},
                "proj-c": {"number": "100003"},
                "rogue-proj": {"number": "100004"},
                "example.com:proj-a": {"number": "100005"},
                "xfstests-cloud": {"number": "999999"},
            },
            "buckets": {},
            "instances": {},
            "firewall_rules": [],
            "pubsub_topics": {},
            "pubsub_subs": {},
            "counters": {},
        }
        self.save_state()
        self._client_seq = 0
        self._vm_seq = 0

    def _write_exec(self, path, content):
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)
        os.chmod(path, 0o755)

    def load_state(self):
        with open(self.state_path, "r", encoding="utf-8") as f:
            self.state = json.load(f)
        return self.state

    def save_state(self):
        with open(self.state_path, "w", encoding="utf-8") as f:
            json.dump(self.state, f, sort_keys=True)

    def create_bucket(self, name="test-bucket", project="proj-a", hns=False,
                      enforce_vm_iam=False):
        self.load_state()
        self.state["buckets"][name] = {
            "project": project,
            "hns": bool(hns),
            "enforce_vm_iam": bool(enforce_vm_iam),
            "objects": {},
            "folders": [],
            "iam": [],
            "notifications": [],
        }
        self.save_state()

    def put_gs_object(self, bucket, obj_name, data, custom_fields=None):
        if isinstance(data, str):
            data = data.encode("utf-8")
        self.load_state()
        binfo = self.state["buckets"][bucket]
        if binfo.get("hns"):
            parts = [p for p in obj_name.rstrip("/").split("/") if p]
            for i in range(1, len(parts)):
                fld = "/".join(parts[:i]) + "/"
                if fld not in binfo["folders"]:
                    binfo["folders"].append(fld)
        prev = binfo["objects"].get(obj_name)
        entry = {
            "data_b64": base64.b64encode(data).decode("ascii"),
            "md5": base64.b64encode(
                __import__("hashlib").md5(data).digest()
            ).decode("ascii"),
            "writes": (prev["writes"] if prev else 0) + 1,
        }
        if custom_fields is not None:
            entry["custom_fields"] = dict(custom_fields)
        binfo["objects"][obj_name] = entry
        self.save_state()

    def get_gs_custom_fields(self, bucket, obj_name):
        self.load_state()
        obj = self.state["buckets"][bucket]["objects"].get(obj_name)
        if not obj:
            return None
        return obj.get("custom_fields")

    def get_gs_object(self, bucket, obj_name):
        self.load_state()
        obj = self.state["buckets"][bucket]["objects"].get(obj_name)
        if not obj:
            return None
        return base64.b64decode(obj["data_b64"])

    def get_gs_writes(self, bucket, obj_name):
        self.load_state()
        obj = self.state["buckets"][bucket]["objects"].get(obj_name)
        return obj["writes"] if obj else 0

    def list_gs_objects(self, bucket):
        self.load_state()
        return sorted(self.state["buckets"][bucket]["objects"].keys())

    def list_gs_folders(self, bucket):
        self.load_state()
        return sorted(self.state["buckets"][bucket].get("folders", []))

    def clear_gcloud_log(self):
        open(self.gcloud_log, "w").close()

    def gcloud_calls(self):
        out = []
        with open(self.gcloud_log, "r", encoding="utf-8") as f:
            for line in f:
                if line.strip():
                    out.append(json.loads(line))
        return out

    def make_client(self, project="proj-a", bucket="test-bucket",
                    version="new", extra_cfg=""):
        """Create an isolated client environment ('new' or 'old')."""
        self._client_seq += 1
        cid = "client-%d-%s" % (self._client_seq, version)
        croot = os.path.join(self.tmp, cid)
        home = os.path.join(croot, "home")
        ctmp = os.path.join(croot, "tmp")
        os.makedirs(os.path.join(home, ".config"), exist_ok=True)
        os.makedirs(ctmp, exist_ok=True)

        src_base = REPO if version == "new" else self.origin.root
        shutil.copytree(os.path.join(src_base, "run-fstests"),
                        os.path.join(croot, "run-fstests"), symlinks=True)
        os.symlink(os.path.join(src_base, "kernel-build"),
                   os.path.join(croot, "kernel-build"))

        dummy_bz = os.path.join(croot, "bzImage")
        with open(dummy_bz, "wb") as f:
            f.write(b"MZ-fake-bzImage")

        cfg_path = os.path.join(home, ".config", "gce-xfstests")
        user_tag = "u%d_%d" % (os.getpid(), self._client_seq)
        # Track old-client hardcoded /tmp/.gce_xfststs.config.$USER-$PROJECT
        self.cleanup_Host_tmp.append(
            "/tmp/.gce_xfststs.config.%s-%s" % (user_tag, project)
        )
        with open(cfg_path, "w", encoding="utf-8") as f:
            f.write(
                'GS_BUCKET=%s\n'
                'GCE_PROJECT=%s\n'
                'GCE_ZONE=us-central1-c\n'
                'GCE_KERNEL=%s\n'
                'GCE_NETWORK=default\n'
                'PRIMARY_FSTYPE=ext4\n'
                '%s\n' % (
                    shlex.quote(bucket),
                    shlex.quote(project),
                    shlex.quote(dummy_bz),
                    extra_cfg,
                )
            )
        return {
            "root": croot,
            "rundir": os.path.join(croot, "run-fstests"),
            "home": home,
            "tmp": ctmp,
            "cfg": cfg_path,
            "user": user_tag,
            "project": project,
            "bucket": bucket,
            "version": version,
        }

    def run_client(self, client, argv, extra_env=None):
        """Invoke gce-xfstests (or a util script if argv[0].startswith('util/'))."""
        env = dict(self.base_env)
        env["HOME"] = client["home"]
        env["TMPDIR"] = client["tmp"]
        env["USER"] = client["user"]
        env["PATH"] = (
            client["rundir"] + ":" + self.bin + ":" +
            self.base_env.get("PATH", "")
        )
        if extra_env:
            env.update(extra_env)
        if argv[0].startswith("util/"):
            prog = os.path.join(client["rundir"], argv[0])
            cmd = [prog] + list(argv[1:])
        else:
            prog = os.path.join(client["rundir"], "gce-xfstests")
            cmd = [prog] + list(argv)
        res = subprocess.run(cmd, env=env, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, stdin=subprocess.DEVNULL)
        return (res.returncode,
                res.stdout.decode("utf-8", "replace"),
                res.stderr.decode("utf-8", "replace"))

    def run_client_bash(self, client, script, extra_env=None):
        env = dict(self.base_env)
        env["HOME"] = client["home"]
        env["TMPDIR"] = client["tmp"]
        env["USER"] = client["user"]
        env["DIR"] = client["rundir"]
        env["XFSTESTS_FLAVOR"] = "gce"
        env["PATH"] = (
            client["rundir"] + ":" + self.bin + ":" +
            self.base_env.get("PATH", "")
        )
        if extra_env:
            env.update(extra_env)
        full = '. "$DIR/util/get-config"\n' + script
        res = subprocess.run([self.tools["bash"], "-c", full], env=env,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             stdin=subprocess.DEVNULL)
        return (res.returncode,
                res.stdout.decode("utf-8", "replace"),
                res.stderr.decode("utf-8", "replace"))

    def stage_vm(self, project="proj-a", bucket="test-bucket", version="new",
                 instance="xfstests-test", attributes=None,
                 fail_project_id=False, caller_sa=""):
        """Stage an isolated test appliance rootfs ('new' or 'old')."""
        self._vm_seq += 1
        vm_key = "vm%d" % self._vm_seq
        vroot = os.path.join(self.tmp, "%s-%s" % (vm_key, version))
        for sub in (
            "usr/local/lib", "usr/local/sbin", "var/lib/gce-xfstests",
            "etc/lighttpd", "etc/rsyslog.d", "root/xfstests_bld",
            "run", "var/log", "var/www", "results", "tmp", "bin", "proc",
        ):
            os.makedirs(os.path.join(vroot, sub), exist_ok=True)

        src_base = REPO if version == "new" else self.origin.root
        src_lib = os.path.join(src_base, "test-appliance", "files", "usr",
                               "local", "lib")
        dst_lib = os.path.join(vroot, "usr", "local", "lib")
        for fn in (
            "gce-funcs", "gce-fetch-gs-files", "gce-setup", "gce-run-batch",
            "gce-ltm.boot", "gce-ltm-batch-watcher", "gce-syslog.conf",
        ):
            sp = os.path.join(src_lib, fn)
            if not os.path.isfile(sp):
                continue
            with open(sp, "r", encoding="utf-8") as f:
                txt = f.read()
            txt = self._relocate_appliance_paths(txt, vroot)
            dp = os.path.join(dst_lib, fn)
            with open(dp, "w", encoding="utf-8") as f:
                f.write(txt)
            os.chmod(dp, os.stat(sp).st_mode)

        # Copy matching run-fstests into /root/xfstests_bld/run-fstests
        shutil.copytree(
            os.path.join(src_base, "run-fstests"),
            os.path.join(vroot, "root", "xfstests_bld", "run-fstests"),
            symlinks=True,
        )
        os.symlink(
            os.path.join(vroot, "root", "xfstests_bld", "run-fstests",
                         "gce-xfstests"),
            os.path.join(vroot, "usr", "local", "sbin", "gce-xfstests"),
        )

        # Appliance helper stubs
        self._write_exec(
            os.path.join(dst_lib, "gce-logger"),
            '#!/bin/sh\nprintf "%%s\\n" "$*" >> %s\n'
            % shlex.quote(os.path.join(vroot, "var", "log", "gce-logger.log"))
        )
        for noop in ("gce-add-metadata", "gce-setup-results",
                     "gce-setup-scratch", "gce-setup-filestore",
                     "gce-setup-pts"):
            self._write_exec(os.path.join(dst_lib, noop), "#!/bin/sh\nexit 0\n")

        with open(os.path.join(vroot, "root", "test-config"), "w") as f:
            f.write("# stub test-config\n")
        with open(os.path.join(vroot, "root", "runtests_utils"), "w") as f:
            f.write("compute_partition_sizes() { return 1; }\n")
        orig_b64 = base64.b64encode(b"smoke").decode("ascii")
        attrs = {"gs_bucket": bucket, "orig_cmdline": orig_b64}
        if attributes:
            attrs.update(attributes)
        with open(os.path.join(vroot, "run", "test-env"), "w") as f:
            f.write('RUN_ON_GCE=yes\nORIG_CMDLINE=%s\n'
                    % shlex.quote(attrs.get("orig_cmdline", orig_b64)))

        for pf in ("diskstats", "meminfo", "lockdep", "lock_stat", "slabinfo",
                   "vmstat", "cpuinfo"):
            with open(os.path.join(vroot, "proc", pf), "w") as f:
                if pf == "cpuinfo":
                    f.write("processor\t: 0\n")
                elif pf == "meminfo":
                    f.write("MemTotal:        8192000 kB\n")
                else:
                    f.write("0\n")
        for lf in ("messages", "syslog", "kern.log"):
            open(os.path.join(vroot, "var", "log", lf), "w").close()

        # Stubs for script, systemctl, logger, pgrep, poweroff
        self._write_exec(
            os.path.join(vroot, "bin", "script"),
            """#!/bin/bash
append=0
cmd=""
while [ $# -gt 0 ]; do
    case "$1" in
        -a) append=1; shift ;;
        -c) cmd="$2"; shift 2 ;;
        *) log="$1"; shift ;;
    esac
done
if [ "$append" = 1 ]; then
    /bin/bash -c "$cmd" >> "$log" 2>&1
else
    /bin/bash -c "$cmd" > "$log" 2>&1
fi
"""
        )
        self._write_exec(
            os.path.join(vroot, "bin", "systemctl"),
            '#!/bin/sh\nprintf "systemctl %%s\\n" "$*" >> %s\n'
            % shlex.quote(os.path.join(vroot, "var", "log", "systemctl.log"))
        )
        self._write_exec(
            os.path.join(vroot, "bin", "logger"),
            '#!/bin/sh\nprintf "%%s\\n" "$*" >> %s\n'
            % shlex.quote(os.path.join(vroot, "var", "log", "logger.log"))
        )
        self._write_exec(os.path.join(vroot, "bin", "pgrep"),
                         "#!/bin/sh\nexit 1\n")
        # poweroff records whether stop-ltm still exists in GCS when invoked!
        poweroff_py = """#!/usr/bin/env python3
import json, os, sys
with open(os.environ["FAKE_GCP_STATE"], "r") as f:
    st = json.load(f)
bobjs = st["buckets"][%r]["objects"]
proj_stop = "ltm-batch/" + %r + "/stop-ltm"
root_stop = "ltm-batch/stop-ltm"
rec = {
    "argv": sys.argv[1:],
    "proj_stop_in_gcs": proj_stop in bobjs,
    "root_stop_in_gcs": root_stop in bobjs,
}
with open(%r, "a") as out:
    out.write(json.dumps(rec) + "\\n")
""" % (bucket, project, os.path.join(vroot, "var", "log", "poweroff.jsonl"))
        self._write_exec(os.path.join(vroot, "bin", "poweroff"), poweroff_py)

        mds_url = self.mds.register_vm(
            vm_key, project_id=project, name=instance, attributes=attrs,
            fail_project_id=fail_project_id)
        return {
            "key": vm_key,
            "root": vroot,
            "mds_url": mds_url,
            "project": project,
            "bucket": bucket,
            "version": version,
            "caller_sa": caller_sa,
        }

    def _relocate_appliance_paths(self, txt, vroot):
        prefixes = [
            "/var/lib/gce-xfstests",
            "/usr/local/lib",
            "/usr/local/sbin",
            "/etc/lighttpd",
            "/etc/rsyslog.d",
            "/root",
            "/run",
            "/var/log",
            "/var/www",
            "/results",
            "/proc",
        ]
        # Protect #!/bin/... shebangs and /bin/true, /bin/rm, /bin/bash, /dev/null
        for p in prefixes:
            txt = re.sub(
                r'(?<![A-Za-z0-9_.-])' + re.escape(p) + r'(?=/|["\'\s;)|]|$)',
                vroot + p,
                txt,
            )
        return txt

    def run_vm_script(self, vm, rel_script, args=None, extra_env=None):
        env = dict(self.base_env)
        env["MDS_PREFIX"] = vm["mds_url"]
        env["GCE_PROJECT"] = ""  # Must come from config or MDS, not host env!
        env["TMPDIR"] = os.path.join(vm["root"], "tmp")
        env["HOME"] = os.path.join(vm["root"], "root")
        env["VM_LOG_DIR"] = os.path.join(vm["root"], "var", "log")
        if vm["caller_sa"]:
            env["GCE_CALLER_SA"] = vm["caller_sa"]
        env["PATH"] = (
            os.path.join(vm["root"], "bin") + ":" +
            os.path.join(vm["root"], "usr", "local", "sbin") + ":" +
            self.bin + ":" + self.base_env.get("PATH", "")
        )
        if extra_env:
            env.update(extra_env)
        spath = os.path.join(vm["root"], rel_script.lstrip("/"))
        cmd = [spath] + list(args or [])
        res = subprocess.run(cmd, env=env, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, stdin=subprocess.DEVNULL)
        return (res.returncode,
                res.stdout.decode("utf-8", "replace"),
                res.stderr.decode("utf-8", "replace"))

    def run_vm_bash(self, vm, body, extra_env=None):
        env = dict(self.base_env)
        env["MDS_PREFIX"] = vm["mds_url"]
        env["GCE_PROJECT"] = ""
        env["TMPDIR"] = os.path.join(vm["root"], "tmp")
        env["HOME"] = os.path.join(vm["root"], "root")
        env["VM_LOG_DIR"] = os.path.join(vm["root"], "var", "log")
        if vm["caller_sa"]:
            env["GCE_CALLER_SA"] = vm["caller_sa"]
        env["PATH"] = (
            os.path.join(vm["root"], "bin") + ":" +
            os.path.join(vm["root"], "usr", "local", "sbin") + ":" +
            self.bin + ":" + self.base_env.get("PATH", "")
        )
        if extra_env:
            env.update(extra_env)
        res = subprocess.run([self.tools["bash"], "-c", body], env=env,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             stdin=subprocess.DEVNULL)
        return (res.returncode,
                res.stdout.decode("utf-8", "replace"),
                res.stderr.decode("utf-8", "replace"))

    def vm_file(self, vm, rel_path):
        p = os.path.join(vm["root"], rel_path.lstrip("/"))
        if not os.path.exists(p):
            return None
        with open(p, "rb") as f:
            return f.read()

    def close(self, keep=False):
        self.mds.close()
        for p in self.cleanup_Host_tmp:
            try:
                os.unlink(p)
            except OSError:
                pass
        if not keep:
            shutil.rmtree(self.tmp, ignore_errors=True)


# ---------------------------------------------------------------------------
# Gating checks: plain CLI test VMs under client/image version skew
# ---------------------------------------------------------------------------

@test
def skew_old_client_new_image(h):
    """origin/master client + new appliance plain test VM works with zero project/ dir"""
    h.create_bucket("bkt", project="proj-a")
    old_cli = h.make_client(project="proj-a", bucket="bkt", version="old")

    # Seed kernel in GCS so smoke --kernel gs://bkt/bzImage skips local upload
    h.put_gs_object("bkt", "bzImage", b"kernel-bytes")

    # Old client automatically runs setup (writing CN-only cert & root config)
    # and launches a plain test VM.
    rc, out, err = h.run_client(
        old_cli, ["smoke", "--kernel", "gs://bkt/bzImage"])
    check(rc == 0, "old client smoke failed (%d):\n%s\n%s" % (rc, out, err))
    check("gce_xfstests.config" in h.list_gs_objects("bkt"),
          "root config missing")
    check(not any(k.startswith("project/") for k in h.list_gs_objects("bkt")),
          "old client unexpectedly created project/ objects")

    # Find launched VM metadata recorded by fake_gcloud
    insts = h.load_state()["instances"]["proj-a"]
    vm_names = [k for k in insts if k.startswith("xfstests-")]
    check(len(vm_names) == 1, "expected 1 test VM, got %r" % vm_names)
    md = insts[vm_names[0]]["metadata"]

    # Boot NEW appliance against the metadata created by the OLD client
    vm = h.stage_vm(project="proj-a", bucket="bkt", version="new",
                    instance=vm_names[0], attributes=md)
    rc, out, err = h.run_vm_script(vm, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, "gce-fetch-gs-files failed: %s\n%s" % (out, err))
    rc, out, err = h.run_vm_script(vm, "/usr/local/lib/gce-setup")
    check(rc == 0, "gce-setup failed: %s\n%s" % (out, err))

    check(h.vm_file(vm, "/var/lib/gce-xfstests/gs_slot_state") == b"owner\n",
          "expected owner slot state")
    check(h.vm_file(vm, "/var/lib/gce-xfstests/gs_bucket_prefix") ==
          b"gs://bkt\n", "expected root prefix")
    check(h.vm_file(vm, "/etc/lighttpd/server.pem") ==
          h.get_gs_object("bkt", "gce-xfstests-server.pem"),
          "server.pem mismatch")
    check(h.vm_file(vm, "/usr/local/lib/gce_xfstests.config") ==
          h.get_gs_object("bkt", "gce_xfstests.config"),
          "local config mismatch")
    log = (h.vm_file(vm, "/var/log/gce-logger.log") or b"").decode()
    check("warning:" not in log, "unexpected warning in gce-logger: %s" % log)


@test
def skew_new_client_old_image(h):
    """New client + origin/master appliance plain test VM reads current root slot"""
    h.create_bucket("bkt", project="proj-a")
    h.put_gs_object("bkt", "bzImage", b"kernel-bytes")
    new_cli = h.make_client(project="proj-a", bucket="bkt", version="new")

    rc, out, err = h.run_client(
        new_cli, ["smoke", "--kernel", "gs://bkt/bzImage"])
    check(rc == 0, "new client smoke failed (%d):\n%s\n%s" % (rc, out, err))

    # Root slot AND project/proj-a/ mirror both populated identically
    for f in ("gce_xfstests.config", "gce-xfstests-cert.pem",
              "gce-xfstests-key.pem", "gce-xfstests-server.pem"):
        root_b = h.get_gs_object("bkt", f)
        mir_b = h.get_gs_object("bkt", "project/proj-a/" + f)
        check(root_b is not None and root_b == mir_b,
              "root vs mirror mismatch for %s" % f)

    insts = h.load_state()["instances"]["proj-a"]
    vm_name = [k for k in insts if k.startswith("xfstests-")][0]
    md = insts[vm_name]["metadata"]

    # Boot OLD (origin/master) appliance
    old_vm = h.stage_vm(project="proj-a", bucket="bkt", version="old",
                        instance=vm_name, attributes=md)
    rc, out, err = h.run_vm_script(old_vm, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, "old gce-fetch-gs-files failed: %s\n%s" % (out, err))
    rc, out, err = h.run_vm_script(old_vm, "/usr/local/lib/gce-setup")
    check(rc == 0, "old gce-setup failed: %s\n%s" % (out, err))

    check(h.vm_file(old_vm, "/usr/local/lib/gce_xfstests.config") ==
          h.get_gs_object("bkt", "gce_xfstests.config"),
          "old VM did not get expected root config")
    check(h.vm_file(old_vm, "/etc/lighttpd/server.pem") ==
          h.get_gs_object("bkt", "gce-xfstests-server.pem"),
          "old VM did not get expected root server.pem")


@test
def skew_new_new_and_mixed_clients(h):
    """New+new steady-state cost (1 cat) and mixed old+new clients on one project"""
    h.create_bucket("bkt", project="proj-a")
    h.put_gs_object("bkt", "bzImage", b"kernel-bytes")
    new_cli = h.make_client(project="proj-a", bucket="bkt", version="new")
    old_cli = h.make_client(project="proj-a", bucket="bkt", version="old")

    rc, out, err = h.run_client(new_cli, ["setup"])
    check(rc == 0, "setup failed:\n%s\n%s" % (out, err))

    # First smoke populates local config cache; second smoke should make
    # only ONE storage call (gcloud storage cat gs://bkt/gce_xfstests.config).
    rc, out, err = h.run_client(
        new_cli, ["smoke", "--kernel", "gs://bkt/bzImage"])
    check(rc == 0, out + err)
    h.clear_gcloud_log()
    rc, out, err = h.run_client(
        new_cli, ["smoke", "--kernel", "gs://bkt/bzImage"])
    check(rc == 0, out + err)
    storage_calls = [
        c["argv"] for c in h.gcloud_calls() if "storage" in c["argv"]
    ]
    # Expect 1 storage cat for slot check + 1 storage ls -b in parse_cli
    cat_calls = [a for a in storage_calls if "cat" in a]
    cp_calls = [a for a in storage_calls if "cp" in a]
    check(len(cat_calls) == 1 and len(cp_calls) == 0,
          "expected 1 storage cat and 0 storage cp on cached run, got: %r"
          % (storage_calls,))

    # Mixed client check: running old client with identical settings must NOT
    # rewrite root gce_xfstests.config (byte-for-byte identical variable list).
    w_before = h.get_gs_writes("bkt", "gce_xfstests.config")
    rc, out, err = h.run_client(
        old_cli, ["smoke", "--kernel", "gs://bkt/bzImage"])
    check(rc == 0, "old client run failed:\n%s\n%s" % (out, err))
    check(h.get_gs_writes("bkt", "gce_xfstests.config") == w_before,
          "old and new clients produced different config bytes and fought!")

    # Now old client changes PRIMARY_FSTYPE=xfs -> updates root -> both old and
    # new VMs read xfs because root is primary for owner!
    with open(old_cli["cfg"], "a", encoding="utf-8") as f:
        f.write("PRIMARY_FSTYPE=xfs\n")
    rc, out, err = h.run_client(
        old_cli, ["smoke", "--kernel", "gs://bkt/bzImage"])
    check(rc == 0, out + err)

    vm_new = h.stage_vm(project="proj-a", bucket="bkt", version="new")
    rc, out, err = h.run_vm_script(vm_new, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, out + err)
    cfg_txt = h.vm_file(vm_new, "/usr/local/lib/gce_xfstests.config").decode()
    check('PRIMARY_FSTYPE="xfs"' in cfg_txt,
          "new VM did not pick up root config written by old client:\n%s"
          % cfg_txt)


@test
def inplace_upgrade_preserves_root_bytes(h):
    """In-place upgrade leaves existing root config & SAN cert untouched and creates mirror"""
    h.create_bucket("bkt", project="proj-a")
    h.put_gs_object("bkt", "bzImage", b"kernel-bytes")
    cli = h.make_client(project="proj-a", bucket="bkt", version="new")

    # Seed bucket with valid SAN certs and matching root config, then delete
    # project/proj-a/ mirror and local cache to simulate pre-upgrade state.
    rc, out, err = h.run_client(cli, ["setup"])
    check(rc == 0, out + err)

    st = h.load_state()
    objs = st["buckets"]["bkt"]["objects"]
    for k in list(objs):
        if k.startswith("project/"):
            del objs[k]
        else:
            objs[k]["writes"] = 1
    h.save_state()

    # Remove new cache file and plant old typo file (.gce_xfststs)
    for fn in os.listdir(cli["tmp"]):
        if fn.startswith(".gce_xfstests.config."):
            os.unlink(os.path.join(cli["tmp"], fn))
    with open(os.path.join(cli["tmp"],
                           ".gce_xfststs.config.%s-proj-a" % cli["user"]),
              "w") as f:
        f.write(h.get_gs_object("bkt", "gce_xfstests.config").decode())

    orig_bytes = {
        f: h.get_gs_object("bkt", f)
        for f in ("gce_xfstests.config", "gce-xfstests-cert.pem",
                  "gce-xfstests-key.pem", "gce-xfstests-server.pem")
    }

    # First run after upgrade + gce-setup-cert
    rc, out, err = h.run_client(
        cli, ["smoke", "--kernel", "gs://bkt/bzImage"])
    check(rc == 0, out + err)
    rc, out, err = h.run_client(cli, ["util/gce-setup-cert"])
    check(rc == 0, out + err)

    for f, expected in orig_bytes.items():
        check(h.get_gs_object("bkt", f) == expected,
              "root %s bytes changed on upgrade!" % f)
        check(h.get_gs_writes("bkt", f) == 1,
              "root %s was rewritten (%d writes)!"
              % (f, h.get_gs_writes("bkt", f)))
        check(h.get_gs_object("bkt", "project/proj-a/" + f) == expected,
              "mirror project/proj-a/%s missing or mismatched!" % f)


@test
def mds_failure_and_root_read_error_fallbacks(h):
    """MDS failure keeps config project; failed root read uses project/$P and never writes root"""
    h.create_bucket("bkt", project="proj-a")
    cli = h.make_client(project="proj-a", bucket="bkt", version="new")
    rc, out, err = h.run_client(cli, ["setup"])
    check(rc == 0, out + err)

    # 1. Metadata server project-id lookup fails -> VM keeps GCE_PROJECT from
    # root config and reads root files cleanly.
    vm_nomds = h.stage_vm(project="proj-a", bucket="bkt", version="new",
                          fail_project_id=True)
    rc, out, err = h.run_vm_script(
        vm_nomds, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, out + err)
    check(h.vm_file(vm_nomds, "/var/lib/gce-xfstests/gce_project") is None,
          "gce_project state file should not exist when MDS fails")
    rc, proj_out, err = h.run_vm_bash(
        vm_nomds,
        '. %s/usr/local/lib/gce-funcs; printf "%%s" "$GCE_PROJECT"'
        % shlex.quote(vm_nomds["root"]))
    check(rc == 0 and proj_out == "proj-a",
          "expected fallback GCE_PROJECT=proj-a from config, got %r" % proj_out)

    # 2. Client root config read fails for non-404 reason (503):
    # Must resolve to GS_SLOT_STATE=error, use project/$P/, and NEVER write root.
    root_cfg_writes = h.get_gs_writes("bkt", "gce_xfstests.config")
    root_cert_writes = h.get_gs_writes("bkt", "gce-xfstests-cert.pem")
    with open(cli["cfg"], "a", encoding="utf-8") as f:
        f.write("PRIMARY_FSTYPE=btrfs\n")
    rc, out, err = h.run_client_bash(
        cli,
        'sync_gs_config\n'
        '"$DIR/util/gce-setup-cert" --force-regen\n'
        'printf "STATE=%s PREFIX=%s MIRROR=%s\\n" '
        '"$GS_SLOT_STATE" "$GS_BUCKET_PREFIX" "$GS_MIRROR_PREFIX"\n',
        extra_env={"FAKE_GCLOUD_FAIL_ROOT_CAT": "1"},
    )
    check(rc == 0, "client failed under root cat error:\n%s\n%s" % (out, err))
    check("STATE=error PREFIX=gs://bkt/project/proj-a MIRROR=" in out, out)
    check(h.get_gs_writes("bkt", "gce_xfstests.config") == root_cfg_writes,
          "client wrote root config after failed root read!")
    check(h.get_gs_writes("bkt", "gce-xfstests-cert.pem") == root_cert_writes,
          "client wrote root cert after failed root read!")
    check(b'PRIMARY_FSTYPE="btrfs"' in
          h.get_gs_object("bkt", "project/proj-a/gce_xfstests.config"),
          "client did not update project/proj-a/gce_xfstests.config")

    # 3. Appliance transient root read failure (fails twice with 503, succeeds
    # on 3rd try) -> retries and still resolves owner.
    vm_retry = h.stage_vm(project="proj-a", bucket="bkt", version="new")
    rc, out, err = h.run_vm_script(
        vm_retry, "/usr/local/lib/gce-fetch-gs-files",
        extra_env={"FAKE_GCLOUD_FAIL_ROOT_CP_COUNT": "2"})
    check(rc == 0, out + err)
    check(h.vm_file(vm_retry, "/var/lib/gce-xfstests/gs_slot_state") ==
          b"owner\n", "expected owner after transient retry recovery")

    # 4. Persistent root read failure (fails all 3 retries) -> falls through to
    # Step 2 (project/proj-a/).
    h.load_state()["counters"].pop("fail_root_cp", None)
    h.save_state()
    vm_step2 = h.stage_vm(project="proj-a", bucket="bkt", version="new")
    rc, out, err = h.run_vm_script(
        vm_step2, "/usr/local/lib/gce-fetch-gs-files",
        extra_env={"FAKE_GCLOUD_FAIL_ROOT_CP_COUNT": "99"})
    check(rc == 0, out + err)
    check(h.vm_file(vm_step2, "/var/lib/gce-xfstests/gs_slot_state") ==
          b"project\n", "expected Step 2 project fallback")
    check(b'PRIMARY_FSTYPE="btrfs"' in
          h.vm_file(vm_step2, "/usr/local/lib/gce_xfstests.config"),
          "expected project/proj-a config in Step 2")


# ---------------------------------------------------------------------------
# Multi-project shared bucket isolation, HNS vs Flat, IAM, and edge cases
# ---------------------------------------------------------------------------

def _run_two_project_isolation(h, hns):
    bkt = "shared-hns" if hns else "shared-flat"
    h.create_bucket(bkt, project="proj-a", hns=hns)
    h.put_gs_object(bkt, "bzImage", b"kernel-bytes")

    cli_a = h.make_client(project="proj-a", bucket=bkt, version="new",
                          extra_cfg="PRIMARY_FSTYPE=ext4")
    cli_b = h.make_client(project="proj-b", bucket=bkt, version="new",
                          extra_cfg="PRIMARY_FSTYPE=xfs")

    # proj-a claims root slot via setup
    rc, out, err = h.run_client(cli_a, ["setup"])
    check(rc == 0, out + err)

    root_cfg_a = h.get_gs_object(bkt, "gce_xfstests.config")
    root_cert_a = h.get_gs_object(bkt, "gce-xfstests-cert.pem")
    w_cfg_a = h.get_gs_writes(bkt, "gce_xfstests.config")
    w_cert_a = h.get_gs_writes(bkt, "gce-xfstests-cert.pem")

    # proj-b runs a plain smoke command WITHOUT manual setup first:
    # gs_config_exists must see root belongs to proj-a and auto-trigger setup!
    rc, out, err = h.run_client(
        cli_b, ["smoke", "--kernel", "gs://%s/bzImage" % bkt])
    check(rc == 0, "proj-b auto-setup + smoke failed:\n%s\n%s" % (out, err))

    # Root files must still belong to proj-a with ZERO extra writes!
    check(h.get_gs_object(bkt, "gce_xfstests.config") == root_cfg_a,
          "proj-b overwrote root config!")
    check(h.get_gs_object(bkt, "gce-xfstests-cert.pem") == root_cert_a,
          "proj-b overwrote root cert!")
    check(h.get_gs_writes(bkt, "gce_xfstests.config") == w_cfg_a,
          "proj-b wrote root config!")
    check(h.get_gs_writes(bkt, "gce-xfstests-cert.pem") == w_cert_a,
          "proj-b wrote root cert!")

    # Verify SANs on proj-a vs proj-b certs
    for proj, path in (("proj-a", "gce-xfstests-cert.pem"),
                       ("proj-b", "project/proj-b/gce-xfstests-cert.pem")):
        pem = h.get_gs_object(bkt, path)
        check(pem is not None, "missing cert at %s" % path)
        p = subprocess.run(
            [h.tools["openssl"], "x509", "-noout", "-ext", "subjectAltName"],
            input=pem, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        san = p.stdout.decode()
        check(("DNS:*.%s.gce-xfstests" % proj) in san,
              "wrong SAN for %s: %s" % (proj, san))

    # Launch LTM and KCS for both projects and queue batch & rc scripts
    for cli, proj, tag in ((cli_a, "proj-a", "A"), (cli_b, "proj-b", "B")):
        rc, out, err = h.run_client(cli, ["launch-ltm"])
        check(rc == 0, "launch-ltm %s failed:\n%s\n%s" % (proj, out, err))
        rc, out, err = h.run_client(cli, ["launch-kcs"])
        check(rc == 0, "launch-kcs %s failed:\n%s\n%s" % (proj, out, err))

        h.put_gs_object(
            bkt, "ltm-rc/%s/01-rc.sh" % proj,
            ('echo "rc-%s:$GCE_PROJECT" > "$VM_LOG_DIR/rc-marker"\n' % tag))
        batch_script = os.path.join(cli["tmp"], "job-%s.sh" % tag)
        with open(batch_script, "w") as f:
            f.write('echo "batch-%s:$GCE_PROJECT" > "$VM_LOG_DIR/batch-marker"\n'
                    % tag)
        rc, out, err = h.run_client(cli, ["ltm-batch", batch_script])
        check(rc == 0, out + err)
        rc, out, err = h.run_client(cli, ["ltm-batch-cmd", "smoke"])
        check(rc == 0, out + err)
        check(("ltm-batch/%s/ltm-batch-cmd" % proj) in h.list_gs_objects(bkt),
              "missing per-project ltm-batch-cmd for %s" % proj)
        # Remove ltm-batch-cmd before executing batch so stub VM doesn't invoke
        # gce-xfstests ltm HTTPS against a non-running Go server.
        st = h.load_state()
        del st["buckets"][bkt]["objects"]["ltm-batch/%s/ltm-batch-cmd" % proj]
        h.save_state()

    # Boot LTM VMs for proj-a and proj-b and verify complete isolation
    for proj, tag, fstype, exp_prefix, exp_state in (
        ("proj-a", "A", "ext4", "gs://%s" % bkt, "owner"),
        ("proj-b", "B", "xfs", "gs://%s/project/proj-b" % bkt, "project"),
    ):
        vm = h.stage_vm(
            project=proj, bucket=bkt, version="new", instance="xfstests-ltm",
            attributes={"gce_xfs_ltm": "true"})
        rc, out, err = h.run_vm_script(
            vm, "/usr/local/lib/gce-fetch-gs-files")
        check(rc == 0, out + err)

        cfg = h.vm_file(vm, "/usr/local/lib/gce_xfstests.config").decode()
        check(('PRIMARY_FSTYPE="%s"' % fstype) in cfg,
              "%s got wrong config:\n%s" % (proj, cfg))
        exp_cert = h.get_gs_object(
            bkt,
            "gce-xfstests-cert.pem" if proj == "proj-a"
            else "project/proj-b/gce-xfstests-cert.pem")
        check(h.vm_file(
            vm, "/root/xfstests_bld/run-fstests/.gce_xfstests_cert_%s.pem"
            % proj) == exp_cert, "%s LTM got wrong cert.pem" % proj)

        config_gce = h.vm_file(
            vm, "/root/xfstests_bld/run-fstests/config.gce").decode()
        check(('GS_BUCKET_PREFIX="%s"' % exp_prefix) in config_gce, config_gce)
        check(('GS_SLOT_STATE="%s"' % exp_state) in config_gce, config_gce)
        check("SKIP_GS_CONFIG_UPDATE=yes" in config_gce, config_gce)

        # Boot LTM (runs ltm-rc/$P with --keep)
        rc, out, err = h.run_vm_script(vm, "/usr/local/lib/gce-ltm.boot")
        check(rc == 0, "gce-ltm.boot %s failed:\n%s\n%s" % (proj, out, err))
        check(h.vm_file(vm, "/var/log/rc-marker") ==
              ("rc-%s:%s\n" % (tag, proj)).encode(),
              "wrong rc-marker on %s" % proj)
        check(("ltm-rc/%s/01-rc.sh" % proj) in h.list_gs_objects(bkt),
              "ltm-rc script was deleted despite --keep!")

        # Run one pass of batch execution + watcher log upload
        rc, out, err = h.run_vm_bash(
            vm,
            '. %s/usr/local/lib/gce-funcs\n'
            '%s/usr/local/lib/gce-run-batch --gce-dir "ltm-batch/$GCE_PROJECT"'
            ' > %s/run/ltm-batch.1 2>&1\n'
            'gcs_cp %s/run/ltm-batch.1 "gs://$GS_BUCKET/ltm-batch-$GCE_PROJECT.log"\n'
            % (shlex.quote(vm["root"]), shlex.quote(vm["root"]),
               shlex.quote(vm["root"]), shlex.quote(vm["root"])))
        check(rc == 0, out + err)
        check(h.vm_file(vm, "/var/log/batch-marker") ==
              ("batch-%s:%s\n" % (tag, proj)).encode(),
              "wrong batch-marker on %s" % proj)
        check(("ltm-batch/%s/job-%s.sh" % (proj, tag))
              not in h.list_gs_objects(bkt),
              "one-shot batch script was not deleted for %s" % proj)
        check(("ltm-batch-%s.log" % proj) in h.list_gs_objects(bkt),
              "missing per-project batch log for %s" % proj)

    check("ltm-batch.log" not in h.list_gs_objects(bkt),
          "legacy un-scoped ltm-batch.log should not exist")


@test
def two_projects_isolation_flat(h):
    """Two projects in one Flat bucket never read or overwrite each other's files"""
    _run_two_project_isolation(h, hns=False)


@test
def two_projects_isolation_hns(h):
    """Two projects in one HNS bucket never read or overwrite each other's files"""
    _run_two_project_isolation(h, hns=True)


@test
def part_c_iam_enforcement_hns_and_cross_project(h):
    """roles/storage.objectUser unlocks cross-project VM reads and HNS folder creation"""
    h.create_bucket("hns-bkt", project="proj-a", hns=True, enforce_vm_iam=True)
    cli_a = h.make_client(project="proj-a", bucket="hns-bkt", version="new")
    cli_b = h.make_client(project="proj-b", bucket="hns-bkt", version="new")

    # Manually seed ONLY legacyBucketWriter (simulating pre-Part-C setup)
    h.put_gs_object("hns-bkt", "gce_xfstests.config",
                    'declare -- GCE_PROJECT="proj-a"\n')
    h.put_gs_object("hns-bkt", "project/proj-b/gce_xfstests.config",
                    'declare -- GCE_PROJECT="proj-b"\n')
    h.put_gs_object("hns-bkt", "project/proj-b/gce-xfstests-server.pem",
                    b"pem-b")
    st = h.load_state()
    st["buckets"]["hns-bkt"]["iam"] = [
        ["serviceAccount:ltm-kcs@proj-a.iam.gserviceaccount.com",
         "roles/storage.legacyBucketWriter"],
        ["serviceAccount:test-vm@proj-b.iam.gserviceaccount.com",
         "roles/storage.legacyBucketWriter"],
    ]
    h.save_state()

    # 1. Same-project VM on HNS fails creating a new folder without objectUser
    vm_a = h.stage_vm(
        project="proj-a", bucket="hns-bkt", version="new",
        caller_sa="ltm-kcs@proj-a.iam.gserviceaccount.com")
    rc, out, err = h.run_vm_bash(
        vm_a,
        '. %s/usr/local/lib/gce-funcs\n'
        'echo hi > %s/tmp/x && gcs_cp %s/tmp/x gs://hns-bkt/results/newdir/x\n'
        % (shlex.quote(vm_a["root"]), shlex.quote(vm_a["root"]),
           shlex.quote(vm_a["root"])))
    check(rc != 0 and "storage.folders.create" in err,
          "expected 403 storage.folders.create without objectUser, got: %s"
          % err)

    # 2. Cross-project VM fails reading objects without objectUser
    vm_b = h.stage_vm(
        project="proj-b", bucket="hns-bkt", version="new",
        caller_sa="test-vm@proj-b.iam.gserviceaccount.com")
    rc, out, err = h.run_vm_bash(
        vm_b,
        '. %s/usr/local/lib/gce-funcs\n'
        'gcs_cat gs://hns-bkt/project/proj-b/gce_xfstests.config\n'
        % shlex.quote(vm_b["root"]))
    check(rc != 0 and "storage.objects.get" in err,
          "expected 403 storage.objects.get on cross-project VM, got: %s" % err)

    # 3. Running gce-do-setup for both projects adds roles/storage.objectUser
    # while keeping roles/storage.legacyBucketWriter, unlocking both operations!
    for cli in (cli_a, cli_b):
        rc, out, err = h.run_client(cli, ["setup"])
        check(rc == 0, "setup failed:\n%s\n%s" % (out, err))

    iam = h.load_state()["buckets"]["hns-bkt"]["iam"]
    for proj in ("proj-a", "proj-b"):
        for sa in ("test-vm", "ltm-kcs", "img-create", "dashboard"):
            member = "serviceAccount:%s@%s.iam.gserviceaccount.com" % (sa, proj)
            check([member, "roles/storage.legacyBucketWriter"] in iam,
                  "missing legacyBucketWriter for %s" % member)
            check([member, "roles/storage.objectUser"] in iam,
                  "missing objectUser for %s" % member)

    rc, out, err = h.run_vm_bash(
        vm_a,
        '. %s/usr/local/lib/gce-funcs\n'
        'echo hi > %s/tmp/x && gcs_cp %s/tmp/x gs://hns-bkt/results/newdir/x\n'
        % (shlex.quote(vm_a["root"]), shlex.quote(vm_a["root"]),
           shlex.quote(vm_a["root"])))
    check(rc == 0, "HNS folder upload still failed after setup: %s" % err)

    rc, out, err = h.run_vm_script(vm_b, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, "cross-project VM fetch failed after setup: %s" % err)
    check(h.vm_file(vm_b, "/var/lib/gce-xfstests/gs_slot_state") ==
          b"project\n", "expected project slot state for proj-b")


@test
def concurrent_setup_and_step2_step3_edge_cases(h):
    """Empty-bucket race, rogue old client overwrite (Step 2), Step 3 fallback, pre-2022 config"""
    h.create_bucket("bkt", project="proj-a")
    cli_a = h.make_client(project="proj-a", bucket="bkt", version="new",
                          extra_cfg="PRIMARY_FSTYPE=ext4")
    cli_b = h.make_client(project="proj-b", bucket="bkt", version="new",
                          extra_cfg="PRIMARY_FSTYPE=xfs")

    # 1. Simulate two setups that both observed an empty bucket (unclaimed):
    # proj-a runs setup, then proj-b runs with GS_SLOT_STATE=unclaimed forced
    # as if resolved concurrently before proj-a wrote root.
    rc, out, err = h.run_client(cli_a, ["setup"])
    check(rc == 0, out + err)
    rc, out, err = h.run_client(
        cli_b, ["setup"],
        extra_env={
            "GS_RESOLVED_FOR": "bkt:proj-b",
            "GS_SLOT_STATE": "unclaimed",
            "GS_BUCKET_PREFIX": "gs://bkt",
            "GS_MIRROR_PREFIX": "gs://bkt/project/proj-b",
        })
    check(rc == 0, out + err)
    # proj-b wrote last and owns root; proj-a's mirror in project/proj-a/ exists.
    # On proj-a's next run, proj-a cleanly switches to project/proj-a/!
    rc, out, err = h.run_client_bash(
        cli_a,
        'resolve_gs_bucket_slot\n'
        'printf "STATE=%s PREFIX=%s\\n" "$GS_SLOT_STATE" "$GS_BUCKET_PREFIX"\n')
    check(rc == 0 and "STATE=project PREFIX=gs://bkt/project/proj-a" in out,
          out + err)

    # 2. Rogue old client overwrites root config while proj-a has mirror ->
    # proj-a new VM rescues config & cert from project/proj-a/ (Step 2).
    old_rogue = h.make_client(project="rogue-proj", bucket="bkt",
                              version="old", extra_cfg="PRIMARY_FSTYPE=f2fs")
    h.put_gs_object("bkt", "bzImage", b"kernel")
    rc, out, err = h.run_client(
        old_rogue, ["smoke", "--kernel", "gs://bkt/bzImage"])
    check(rc == 0, out + err)
    check(b'GCE_PROJECT="rogue-proj"' in
          h.get_gs_object("bkt", "gce_xfstests.config"),
          "rogue client did not overwrite root")

    vm_a = h.stage_vm(project="proj-a", bucket="bkt", version="new")
    rc, out, err = h.run_vm_script(vm_a, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, out + err)
    check(h.vm_file(vm_a, "/var/lib/gce-xfstests/gs_slot_state") ==
          b"project\n", "expected Step 2 rescue")
    check(b'PRIMARY_FSTYPE="ext4"' in
          h.vm_file(vm_a, "/usr/local/lib/gce_xfstests.config"),
          "proj-a VM did not get its mirrored ext4 config")
    check(h.vm_file(vm_a, "/etc/lighttpd/server.pem") ==
          h.get_gs_object("bkt", "project/proj-a/gce-xfstests-server.pem"),
          "proj-a VM did not get its mirrored server.pem")

    # 3. Appliance Step 3 fallback: proj-c has NO project/proj-c/ mirror while
    # root names rogue-proj -> logs warning, uses root config, forces GCE_PROJECT=proj-c.
    vm_c = h.stage_vm(project="proj-c", bucket="bkt", version="new")
    rc, out, err = h.run_vm_script(vm_c, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, out + err)
    check(h.vm_file(vm_c, "/var/lib/gce-xfstests/gs_slot_state") ==
          b"fallback\n", "expected Step 3 fallback")
    cfg_c = h.vm_file(vm_c, "/usr/local/lib/gce_xfstests.config").decode()
    check('GCE_PROJECT="proj-c"' in cfg_c and "rogue-proj" not in cfg_c,
          "Step 3 did not force GCE_PROJECT=proj-c:\n%s" % cfg_c)
    log_c = (h.vm_file(vm_c, "/var/log/gce-logger.log") or b"").decode()
    check("warning: falling back to root gce_xfstests.config" in log_c, log_c)

    # 4. Pre-2022 root config with NO GCE_PROJECT line -> treated as owned by
    # reader on both client and appliance.
    h.put_gs_object("bkt", "gce_xfstests.config",
                    'declare -- PRIMARY_FSTYPE="ext4"\n')
    rc, out, err = h.run_client_bash(
        cli_a,
        'resolve_gs_bucket_slot\nprintf "STATE=%s\\n" "$GS_SLOT_STATE"\n')
    check(rc == 0 and "STATE=owner" in out, out + err)
    vm_pre2022 = h.stage_vm(project="proj-a", bucket="bkt", version="new")
    rc, out, err = h.run_vm_script(
        vm_pre2022, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, out + err)
    check(h.vm_file(vm_pre2022, "/var/lib/gce-xfstests/gs_slot_state") ==
          b"owner\n", "expected owner on pre-2022 root config")
    check(b'GCE_PROJECT="proj-a"' in
          h.vm_file(vm_pre2022, "/usr/local/lib/gce_xfstests.config"),
          "appliance did not inject GCE_PROJECT into pre-2022 local config")


@test
def domain_scoped_and_invalid_project_ids(h):
    """Domain-scoped IDs (example.com:proj-a) work everywhere; invalid IDs fail fast"""
    h.create_bucket("bkt", project="proj-a", hns=True)
    h.put_gs_object("bkt", "bzImage", b"kernel")

    # proj-a owns root so example.com:proj-a exercises per-project paths everywhere
    cli_a = h.make_client(project="proj-a", bucket="bkt", version="new")
    rc, out, err = h.run_client(cli_a, ["setup"])
    check(rc == 0, out + err)

    dom_proj = "example.com:proj-a"
    cli_dom = h.make_client(project=dom_proj, bucket="bkt", version="new")
    rc, out, err = h.run_client(cli_dom, ["get-project"])
    check(rc == 0 and out.strip() == dom_proj, out + err)

    rc, out, err = h.run_client(cli_dom, ["setup"])
    check(rc == 0, "domain-scoped setup failed:\n%s\n%s" % (out, err))
    check(("project/%s/gce_xfstests.config" % dom_proj)
          in h.list_gs_objects("bkt"), "missing domain-scoped config")
    pem = h.get_gs_object("bkt", "project/%s/gce-xfstests-cert.pem" % dom_proj)
    p = subprocess.run(
        [h.tools["openssl"], "x509", "-noout", "-ext", "subjectAltName"],
        input=pem, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    check("DNS:*.example.com-proj-a.gce-xfstests" in p.stdout.decode(),
          p.stdout.decode())

    rc, out, err = h.run_client(cli_dom, ["ltm-batch-cmd", "smoke"])
    check(rc == 0, out + err)
    check(("ltm-batch/%s/ltm-batch-cmd" % dom_proj)
          in h.list_gs_objects("bkt"), "missing domain-scoped batch file")

    vm_dom = h.stage_vm(project=dom_proj, bucket="bkt", version="new")
    rc, out, err = h.run_vm_script(vm_dom, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, out + err)
    check(h.vm_file(vm_dom, "/var/lib/gce-xfstests/gs_bucket_prefix") ==
          ("gs://bkt/project/%s\n" % dom_proj).encode(),
          "domain-scoped prefix mismatch")

    # Invalid / path-traversal GCE_PROJECT values must fail fast on Part A paths
    for bad in ("../evil", "a/b", "-flag", ".."):
        bad_cli = h.make_client(project=bad, bucket="bkt", version="new")
        for subcmd in (["ltm-batch-cmd", "smoke"], ["stop-ltm"]):
            rc, out, err = h.run_client(bad_cli, subcmd)
            check(rc != 0 and "Invalid or empty GCE_PROJECT" in err,
                  "accepted bad project %r on %r: rc=%d err=%s"
                  % (bad, subcmd, rc, err))
        vm_bad = h.stage_vm(project=bad, bucket="bkt", version="new")
        rc, out, err = h.run_vm_script(vm_bad, "/usr/local/lib/gce-run-batch")
        check(rc != 0 and "Invalid or empty GCE_PROJECT" in err,
              "gce-run-batch accepted bad project %r" % bad)


def _run_batch_and_rc_checks(h, hns):
    bkt = "batch-hns" if hns else "batch-flat"
    h.create_bucket(bkt, project="proj-a", hns=hns)
    cli_a = h.make_client(project="proj-a", bucket=bkt, version="new")
    cli_b = h.make_client(project="proj-b", bucket=bkt, version="new")
    rc, out, err = h.run_client(cli_a, ["setup"])
    check(rc == 0, out + err)
    rc, out, err = h.run_client(cli_b, ["setup"])
    check(rc == 0, out + err)

    vm = h.stage_vm(project="proj-a", bucket=bkt, version="new",
                    instance="xfstests-ltm",
                    attributes={"gce_xfs_ltm": "true"})
    rc, out, err = h.run_vm_script(vm, "/usr/local/lib/gce-fetch-gs-files")
    check(rc == 0, out + err)

    # 1. Empty batch/rc directory BEFORE any object ever existed -> quiet exit 0
    rc, out, err = h.run_vm_script(vm, "/usr/local/lib/gce-run-batch")
    check(rc == 0 and err == "",
          "empty batch failed (%s): rc=%d err=%r" % (bkt, rc, err))

    # 2. Upload & run batch scripts (including trailing slash, Flat 0-byte
    # placeholder, nested subdir that must be ignored, and stop-ltm deleted
    # BEFORE poweroff runs!)
    if not hns:
        # Console-created 0-byte directory placeholder on Flat bucket
        h.put_gs_object(bkt, "ltm-batch/proj-a/", b"")
    h.put_gs_object(
        bkt, "ltm-batch/proj-a/01 first.sh",
        'echo "one" >> "$VM_LOG_DIR/order.txt"\n')
    h.put_gs_object(
        bkt, "ltm-batch/proj-a/subdir/ignored.sh",
        'echo "SUBDIR_RAN" >> "$VM_LOG_DIR/order.txt"\n')
    h.put_gs_object(
        bkt, "ltm-batch/proj-b/other.sh",
        'echo "OTHER_PROJ_RAN" >> "$VM_LOG_DIR/order.txt"\n')
    rc, out, err = h.run_client(cli_a, ["stop-ltm"])
    check(rc == 0, out + err)
    check("ltm-batch/proj-a/stop-ltm" in h.list_gs_objects(bkt),
          "stop-ltm was not uploaded to ltm-batch/proj-a/stop-ltm")

    # Pass explicit trailing '/' on --gce-dir to verify stripping
    rc, out, err = h.run_vm_script(
        vm, "/usr/local/lib/gce-run-batch",
        args=["--gce-dir", "ltm-batch/proj-a/"])
    check(rc == 0, "gce-run-batch failed: %s\n%s" % (out, err))
    check(h.vm_file(vm, "/var/log/order.txt") == b"one\n",
          "unexpected execution order/isolation: %r"
          % h.vm_file(vm, "/var/log/order.txt"))

    # Verify stop-ltm was removed from GCS BEFORE poweroff executed!
    poff_raw = h.vm_file(vm, "/var/log/poweroff.jsonl")
    check(poff_raw is not None, "stop-ltm did not invoke poweroff!")
    poff = json.loads(poff_raw.decode().splitlines()[0])
    check(poff["argv"] == ["-f"], "unexpected poweroff args: %r" % poff)
    check(poff["proj_stop_in_gcs"] is False,
          "stop-ltm was STILL in GCS when poweroff ran!")

    # 3. Empty directory AFTER deleting the last object!
    # Remove the Flat placeholder so ltm-batch/proj-a has 0 objects (while on
    # HNS the persistent empty folder 'ltm-batch/proj-a/' still exists!).
    if not hns:
        st = h.load_state()
        st["buckets"][bkt]["objects"].pop("ltm-batch/proj-a/", None)
        h.save_state()
    else:
        check("ltm-batch/proj-a/" in h.list_gs_folders(bkt),
              "expected persistent empty HNS folder ltm-batch/proj-a/")
        # Verify fake_gcloud models HNS ls=0 vs rsync=1 on empty folder
        rc_ls, _, _ = h.run_client_bash(
            cli_a, 'gcs_exists "gs://%s/ltm-batch/proj-a"' % bkt)
        check(rc_ls == 0, "gcloud storage ls on empty HNS folder must exit 0")

    rc, out, err = h.run_vm_script(vm, "/usr/local/lib/gce-run-batch")
    check(rc == 0 and err == "",
          "post-delete empty batch failed (%s): rc=%d err=%r" % (bkt, rc, err))

    # 4. Legacy root file warnings + gcloud storage mv migration + launch-ltm
    # stop-ltm cleanup rules (owner cleans root stop-ltm, non-owner does NOT).
    # First: with ONLY ltm-batch/proj-b/other.sh (in a subdirectory), NO legacy
    # warning should fire!
    rc, out, err = h.run_vm_script(vm, "/usr/local/lib/gce-ltm.boot")
    check(rc == 0, out + err)
    log1 = (h.vm_file(vm, "/var/log/gce-logger.log") or b"").decode()
    check("legacy scripts" not in log1,
          "false-positive legacy warning on subdirectory: %s" % log1)

    # Plant legacy root file ltm-rc/legacy.sh and stale root ltm-batch/stop-ltm
    h.put_gs_object(
        bkt, "ltm-rc/legacy.sh", 'echo migrated > "$VM_LOG_DIR/mig"\n')
    h.put_gs_object(bkt, "ltm-batch/stop-ltm", "poweroff -f\n")

    # Non-owner (proj-b) launch-ltm must warn about legacy files, must NOT
    # delete root ltm-batch/stop-ltm (owned by proj-a!).
    rc, out, err = h.run_client(cli_b, ["launch-ltm"])
    check(rc == 0, out + err)
    check("gcloud storage mv" in out,
          "missing migration notice on launch-ltm:\n%s" % out)
    check("ltm-batch/stop-ltm" in h.list_gs_objects(bkt),
          "non-owner proj-b deleted root ltm-batch/stop-ltm!")

    # Owner (proj-a) launch-ltm DOES clean stale root ltm-batch/stop-ltm
    rc, out, err = h.run_client(cli_a, ["launch-ltm"])
    check(rc == 0, out + err)
    check("ltm-batch/stop-ltm" not in h.list_gs_objects(bkt),
          "owner proj-a did not clean stale root ltm-batch/stop-ltm!")

    # Booting LTM logs 1-line warning for ltm-rc/legacy.sh and does NOT run it
    rc, out, err = h.run_vm_script(vm, "/usr/local/lib/gce-ltm.boot")
    check(rc == 0, out + err)
    log2 = (h.vm_file(vm, "/var/log/gce-logger.log") or b"").decode()
    check("warning: legacy scripts in gs://%s/ltm-rc/ are ignored" % bkt
          in log2, log2)
    check(h.vm_file(vm, "/var/log/mig") is None,
          "legacy root ltm-rc script should not have executed!")

    # Run the documented migration command and verify it runs on next boot!
    rc, out, err = h.run_client_bash(
        cli_a,
        'gcloud storage mv "gs://$GS_BUCKET/ltm-rc/*" '
        '"gs://$GS_BUCKET/ltm-rc/$GCE_PROJECT/"\n')
    check(rc == 0, "migration mv failed: %s\n%s" % (out, err))
    rc, out, err = h.run_vm_script(vm, "/usr/local/lib/gce-ltm.boot")
    check(rc == 0, out + err)
    check(h.vm_file(vm, "/var/log/mig") == b"migrated\n",
          "migrated ltm-rc script did not execute on boot!")


@test
def batch_and_rc_semantics_flat(h):
    """Empty dirs, trailing /, non-recursive rsync, stop-ltm pre-delete, migration (Flat)"""
    _run_batch_and_rc_checks(h, hns=False)


@test
def batch_and_rc_semantics_hns(h):
    """Empty dirs, persistent HNS folders, stop-ltm pre-delete, migration (HNS)"""
    _run_batch_and_rc_checks(h, hns=True)


def _make_fake_x86_bzimage(path, ver_str="6.12.0-rc1-00012-gabcdef123456"):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    buf = bytearray(1024)
    buf[510:512] = (0xAA55).to_bytes(2, "little")
    buf[514:518] = b"HdrS"
    buf[518:520] = (0x020F).to_bytes(2, "little")
    buf[526:530] = (0x60).to_bytes(4, "little")
    buf[529] = 1
    payload = ("%s (builder@host) #1 SMP PREEMPT_DYNAMIC\x00"
               % ver_str).encode("ascii")
    buf[0x260:0x260 + len(payload)] = payload
    with open(path, "wb") as f:
        f.write(buf)


def _make_fake_arm64_image(path):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    buf = bytearray(256)
    buf[0x38:0x3C] = (0x644D5241).to_bytes(4, "little")
    with open(path, "wb") as f:
        f.write(buf)


@test
def kernel_file_info_local_and_gcs_formats(h):
    """get_kernel_file_info handles raw kernels (.git_version, kernel.release, bzImage) and GCS JSON"""
    h.create_bucket("bkt", project="proj-a")
    cli = h.make_client(project="proj-a", bucket="bkt", version="new")
    rc, out, err = h.run_client(cli, ["setup"])
    check(rc == 0, out + err)

    # 1. x86 bzImage in build tree: empty .git_version falls back to kernel.release
    bld_x86 = os.path.join(cli["tmp"], "bld-x86")
    bz_in_tree = os.path.join(bld_x86, "arch", "x86", "boot", "bzImage")
    _make_fake_x86_bzimage(bz_in_tree, "6.13.0-header")
    with open(os.path.join(bld_x86, ".config"), "w") as f:
        f.write("# Linux/x86_64 6.13.0 Kernel Configuration\n")
    with open(os.path.join(bld_x86, ".git_version"), "w") as f:
        f.write("")  # 0-byte .git_version (e.g. failed git describe)
    krel_path = os.path.join(bld_x86, "include", "config", "kernel.release")
    os.makedirs(os.path.dirname(krel_path), exist_ok=True)
    with open(krel_path, "w") as f:
        f.write("6.13.0-rc2-00005-g112233445566\n")

    rc, out, err = h.run_client(cli, ["get-kernel-info", bz_in_tree])
    check(rc == 0, out + err)
    check("KERNEL_ARCH=amd64\nKERNEL_VERSION=6.13.0-rc2-00005-g112233445566\n"
          == out, "unexpected kernel.release output: %r" % out)

    # Relative path invocation from inside build tree resolves parent dirs
    rel_env_script = (
        'cd %s && "$DIR/gce-xfstests" get-kernel-info arch/x86/boot/bzImage\n'
        % shlex.quote(bld_x86)
    )
    rc, out, err = h.run_client_bash(cli, rel_env_script)
    check(rc == 0, out + err)
    check("KERNEL_ARCH=amd64\nKERNEL_VERSION=6.13.0-rc2-00005-g112233445566\n"
          == out, "unexpected relative bzImage output: %r" % out)

    # Non-empty .git_version takes precedence
    with open(os.path.join(bld_x86, ".git_version"), "w") as f:
        f.write("v6.13-rc2-5-g112233445566\n")
    rc, out, err = h.run_client(cli, ["get-kernel-info", bz_in_tree])
    check(rc == 0, out + err)
    check("KERNEL_ARCH=amd64\nKERNEL_VERSION=v6.13-rc2-5-g112233445566\n"
          == out, "unexpected .git_version output: %r" % out)

    # 2. Standalone x86 bzImage outside any build tree extracts version from header
    standalone_bz = os.path.join(cli["tmp"], "standalone-bzImage")
    _make_fake_x86_bzimage(standalone_bz, "6.12.7-standalone")
    rc, out, err = h.run_client(cli, ["get-kernel-info", standalone_bz])
    check(rc == 0, out + err)
    check("KERNEL_ARCH=x86\nKERNEL_VERSION=6.12.7-standalone\n" == out,
          "unexpected standalone bzImage output: %r" % out)

    # 3. ARM64 Image in build tree without .git_version uses kernel.release
    bld_arm = os.path.join(cli["tmp"], "bld-arm64")
    arm_img = os.path.join(bld_arm, "arch", "arm64", "boot", "Image")
    _make_fake_arm64_image(arm_img)
    arm_krel = os.path.join(bld_arm, "include", "config", "kernel.release")
    os.makedirs(os.path.dirname(arm_krel), exist_ok=True)
    with open(arm_krel, "w") as f:
        f.write("6.13.0-arm64\n")
    rc, out, err = h.run_client(cli, ["get-kernel-info", arm_img])
    check(rc == 0, out + err)
    check("KERNEL_ARCH=arm64\nKERNEL_VERSION=6.13.0-arm64\n" == out,
          "unexpected arm64 Image output: %r" % out)

    # 4. GCS JSON metadata: YAML-ambiguous '6.12', ver-only, legacy arch=/ver=, empty
    h.put_gs_object("bkt", "k-float", b"x",
                    custom_fields={"arch": "amd64", "ver": "6.12"})
    rc, out, err = h.run_client(cli, ["get-kernel-info", "gs://bkt/k-float"])
    check(rc == 0 and out == "KERNEL_ARCH=amd64\nKERNEL_VERSION=6.12\n",
          "unexpected float-like ver output: %r (%s)" % (out, err))

    h.put_gs_object("bkt", "k-ver-only", b"x",
                    custom_fields={"ver": "1:6.12.0-1"})
    rc, out, err = h.run_client(cli, ["get-kernel-info", "gs://bkt/k-ver-only"])
    check(rc == 0 and out == "KERNEL_ARCH=\nKERNEL_VERSION=1:6.12.0-1\n",
          "unexpected ver-only output: %r (%s)" % (out, err))

    h.put_gs_object("bkt", "k-legacy", b"x",
                    custom_fields={"arch=arm64": "", "ver=6.1.0": ""})
    rc, out, err = h.run_client(cli, ["get-kernel-info", "gs://bkt/k-legacy"])
    check(rc == 0 and out == "KERNEL_ARCH=arm64\nKERNEL_VERSION=6.1.0\n",
          "unexpected legacy metadata output: %r (%s)" % (out, err))

    h.put_gs_object("bkt", "k-empty", b"x", custom_fields={})
    rc, out, err = h.run_client(cli, ["get-kernel-info", "gs://bkt/k-empty"])
    check(rc == 0 and out == "KERNEL_ARCH=\nKERNEL_VERSION=\n",
          "unexpected empty metadata output: %r (%s)" % (out, err))


@test
def ltm_launch_sets_and_repairs_kernel_metadata(h):
    """gce-xfstests ltm tags local kernel uploads and repairs untagged cached kernels"""
    h.create_bucket("bkt", project="proj-a")
    fake_curl = os.path.join(h.bin, "fake-ltm-curl")
    curl_log = os.path.join(h.tmp, "ltm_curl.jsonl")
    h._write_exec(
        fake_curl,
        '#!/usr/bin/env python3\n'
        'import json, sys\n'
        'argv = sys.argv[1:]\n'
        'url = argv[-1]\n'
        'cookie = None\n'
        'body = None\n'
        'for i, a in enumerate(argv):\n'
        '    if a == "-c" and i + 1 < len(argv):\n'
        '        cookie = argv[i + 1]\n'
        '    if a == "-d" and i + 1 < len(argv):\n'
        '        body = argv[i + 1]\n'
        'if cookie:\n'
        '    with open(cookie, "w") as f:\n'
        '        f.write("a.proj-a.gce-xfstests\\tFALSE\\t/\\tTRUE\\t0\\ts\\t1\\n")\n'
        'with open(%r, "a") as f:\n'
        '    f.write(json.dumps({"url": url, "body": body}) + "\\n")\n'
        'if url.endswith("/login"):\n'
        '    print(json.dumps({"status": True, "msg": "Logged in"}))\n'
        'else:\n'
        '    print(json.dumps({"status": True, "test_id": "t1", "msg": "Launching tests"}))\n'
        % (curl_log,)
    )

    cli = h.make_client(
        project="proj-a", bucket="bkt", version="new",
        extra_cfg="GCE_CURL=%s" % shlex.quote(fake_curl)
    )
    # Replace dummy_bz with a valid x86 bzImage in a build tree so arch+ver are known
    bld_dir = os.path.join(cli["tmp"], "bld-cfg")
    cfg_bz = os.path.join(bld_dir, "arch", "x86", "boot", "bzImage")
    _make_fake_x86_bzimage(cfg_bz, "6.14.0-rc1-cfg")
    with open(os.path.join(bld_dir, ".config"), "w") as f:
        f.write("# Linux/x86_64 6.14.0 Kernel Configuration\n")
    with open(cli["cfg"], "a", encoding="utf-8") as f:
        f.write("GCE_KERNEL=%s\n" % shlex.quote(cfg_bz))

    rc, out, err = h.run_client(cli, ["setup"])
    check(rc == 0, out + err)

    ltm_inst = os.path.join(cli["rundir"], ".ltm_instance_proj-a")
    with open(ltm_inst, "w", encoding="utf-8") as f:
        f.write(
            'GCE_LTM_SERVER_CERT=%s/.gce_xfstests_cert_proj-a.pem\n'
            'GCE_LTM_NAME=xfstests-ltm\n'
            'GCE_LTM_EXT_IP=127.0.0.1\n'
            'GCE_LTM_PWD=secret\n' % cli["rundir"]
        )

    # 1. Explicit --kernel <local> uploads <testrunid>-onerun.bzImage WITH metadata
    override_bz = os.path.join(cli["tmp"], "override-bzImage")
    _make_fake_x86_bzimage(override_bz, "6.14.2-override")
    rc, out, err = h.run_client(
        cli, ["ltm", "--testrunid", "run1", "--kernel", override_bz,
              "-c", "ext4/4k", "generic/001"])
    check(rc == 0, "ltm --kernel failed:\n%s\n%s" % (out, err))
    check(h.get_gs_custom_fields("bkt", "run1-onerun.bzImage") == {
        "arch": "amd64", "ver": "6.14.2-override"
    }, "expected metadata on onerun kernel, got: %r"
       % (h.get_gs_custom_fields("bkt", "run1-onerun.bzImage"),))
    rc, out, err = h.run_client(
        cli, ["get-kernel-info", "gs://bkt/run1-onerun.bzImage"])
    check(rc == 0 and out == "KERNEL_ARCH=amd64\nKERNEL_VERSION=6.14.2-override\n",
          "get-kernel-info on onerun failed: %r" % out)

    # 2. Seed gs://bkt/bzImage with matching bytes of cfg_bz but NO custom_fields
    #    (simulating an earlier untagged upload where LOCAL_HASH == REMOTE_HASH).
    with open(cfg_bz, "rb") as f:
        cfg_bytes = f.read()
    h.put_gs_object("bkt", "bzImage", cfg_bytes, custom_fields={})
    w_before = h.get_gs_writes("bkt", "bzImage")

    rc, out, err = h.run_client(
        cli, ["ltm", "--testrunid", "run2", "-c", "ext4/4k", "generic/001"])
    check(rc == 0, "ltm default kernel failed:\n%s\n%s" % (out, err))
    check(h.get_gs_writes("bkt", "bzImage") == w_before,
          "expected metadata patch without re-uploading bzImage bytes")
    check(h.get_gs_custom_fields("bkt", "bzImage") == {
        "arch": "amd64", "ver": "6.14.0-rc1-cfg"
    }, "expected repaired metadata on gs://bkt/bzImage, got: %r"
       % (h.get_gs_custom_fields("bkt", "bzImage"),))

    # 3. Second ltm launch with already-tagged cached kernel should NOT call objects update
    h.clear_gcloud_log()
    rc, out, err = h.run_client(
        cli, ["ltm", "--testrunid", "run3", "-c", "ext4/4k", "generic/001"])
    check(rc == 0, out + err)
    update_calls = [
        c["argv"] for c in h.gcloud_calls()
        if "objects" in c["argv"] and "update" in c["argv"]
    ]
    check(len(update_calls) == 0,
          "unexpected objects update when metadata already matched: %r"
          % (update_calls,))


def main():
    global verbose
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("-v", "--verbose", action="store_true")
    ap.add_argument("-k", metavar="SUBSTRING", default="",
                    help="run only tests matching SUBSTRING")
    ap.add_argument("--list", action="store_true", help="list tests and exit")
    ap.add_argument("--keep", action="store_true",
                    help="keep temporary directories")
    ap.add_argument("--repo", default=DEFAULT_REPO,
                    help="path to xfstests-bld repository")
    args = ap.parse_args()
    verbose = args.verbose

    if args.list:
        for t in TESTS:
            print("%-44s %s" % (t.__name__, (t.__doc__ or "").strip()))
        return 0

    resolve_repo(args.repo)
    tools = require_tools()
    selected = [t for t in TESTS if args.k in t.__name__]
    if not selected:
        sys.stderr.write("no tests matched %r\n" % args.k)
        return 2

    cache_tmp = tempfile.mkdtemp(prefix="sharedbkt-origin.")
    failed = []
    try:
        origin_cache = OriginArchiveCache(tools["git"], cache_tmp)
        for t in selected:
            sys.stdout.write("%-44s %s\n"
                             % (t.__name__, (t.__doc__ or "").strip()))
            sys.stdout.flush()
            t0 = time.time()
            h = Harness(tools, origin_cache)
            ok = False
            try:
                t(h)
                ok = True
                print("    PASS (%.1fs)" % (time.time() - t0))
            except Exception:
                failed.append(t.__name__)
                sys.stdout.write("    FAIL:\n")
                traceback.print_exc(file=sys.stdout)
                print("    kept %s" % h.tmp)
            finally:
                h.close(keep=(args.keep or not ok))
    finally:
        if not args.keep:
            shutil.rmtree(cache_tmp, ignore_errors=True)

    if failed:
        print("\n%d failed: %s" % (len(failed), ", ".join(failed)))
        return 1
    print("\nall %d passed" % len(selected))
    return 0


if __name__ == "__main__":
    sys.exit(main())
