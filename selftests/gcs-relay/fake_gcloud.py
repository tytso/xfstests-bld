#!/usr/bin/env python3
"""A fake "gcloud" for testing, which talks to fake_gcp.py.

Only the handful of commands used by gce-gcs-curl (and by the shell
functions around it) are implemented:

    gcloud [--account A] [--project P] storage cp - gs://BUCKET/OBJECT
    gcloud ... storage cat gs://BUCKET/OBJECT
    gcloud ... storage rm gs://BUCKET/OBJECT
    gcloud ... storage ls gs://BUCKET/PREFIX
    gcloud ... compute -q instances add-metadata INSTANCE --zone ZONE \
                --metadata KEY=VALUE
    gcloud ... compute instances describe ...       (always "not found")

Environment:
    FAKE_GCP_URL        base URL of fake_gcp.py (required)
    FAKE_GCLOUD_LOG     append one JSON line per invocation to this file
    FAKE_GCLOUD_DELAY   seconds to sleep at startup (default 0.3), to
                        mimic how slow starting the real gcloud is
    FAKE_GCLOUD_DELAY_<OP>  override for one operation (CP, CAT, RM,
                        ADD_METADATA)
    FAKE_GCLOUD_FAIL    comma-separated operations which should fail
    FAKE_GCLOUD_LOCK    path to a lockfile held shared while running
    FAKE_GCLOUD_EPOCH   test epoch header sent to fake_gcp.py
"""

import fcntl
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

BASE = os.environ.get("FAKE_GCP_URL", "")
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def http(method, path, query=None, data=None, auth=True):
    url = BASE + path
    if query:
        url += "?" + urllib.parse.urlencode(query)
    headers = {"X-Fake-Gcloud": "1"}
    ep = os.environ.get("FAKE_GCLOUD_EPOCH")
    if ep is not None:
        headers["X-Fake-Epoch"] = ep
    if auth:
        req = urllib.request.Request(
            BASE + "/computeMetadata/v1/instance/service-accounts/default/token",
            headers={"Metadata-Flavor": "Google", "X-Fake-Gcloud": "1"})
        tok = json.loads(OPENER.open(req).read().decode())["access_token"]
        headers["Authorization"] = "Bearer " + tok
    req = urllib.request.Request(url, data=data, headers=headers,
                                 method=method)
    try:
        with OPENER.open(req) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()


def split_gs(url):
    assert url.startswith("gs://"), url
    bucket, _, name = url[5:].partition("/")
    return bucket, name


def obj_path(bucket, name):
    return "/storage/v1/b/%s/o/%s" % (urllib.parse.quote(bucket, safe=""),
                                      urllib.parse.quote(name, safe=""))


def main():
    lock_fh = None
    lock_path = os.environ.get("FAKE_GCLOUD_LOCK")
    if lock_path:
        lock_fh = open(lock_path, "a+")
        fcntl.flock(lock_fh.fileno(), fcntl.LOCK_SH)
    argv = sys.argv[1:]
    if os.environ.get("FAKE_GCLOUD_LOG"):
        with open(os.environ["FAKE_GCLOUD_LOG"], "a") as f:
            f.write(json.dumps({"time": time.time(), "argv": argv}) + "\n")

    # Strip the global flags
    args = []
    flags = {}
    i = 0
    while i < len(argv):
        a = argv[i]
        if a in ("--account", "--project", "--zone", "--metadata",
                 "--format"):
            flags[a[2:]] = argv[i + 1]
            i += 2
            continue
        if a.startswith("--") and "=" in a:
            k, v = a[2:].split("=", 1)
            flags[k] = v
        elif a in ("-q", "--quiet"):
            pass
        else:
            args.append(a)
        i += 1

    op = "-".join(args[:3]) if args[:1] == ["compute"] else "-".join(args[:2])
    short = {"storage-cp": "cp", "storage-cat": "cat", "storage-rm": "rm",
             "storage-ls": "ls",
             "compute-instances-add-metadata": "add-metadata",
             "compute-instances-describe": "describe"}.get(op, op)

    delay = os.environ.get("FAKE_GCLOUD_DELAY_" +
                           short.upper().replace("-", "_"))
    if delay is None:
        delay = os.environ.get("FAKE_GCLOUD_DELAY", "0.3")
    time.sleep(float(delay))

    if short in os.environ.get("FAKE_GCLOUD_FAIL", "").split(","):
        sys.stderr.write("ERROR: (gcloud.%s) injected failure: you do not "
                         "have permission\n" % op.replace("-", "."))
        return 1

    if not BASE:
        sys.stderr.write("fake gcloud: FAKE_GCP_URL is not set\n")
        return 2

    if short == "cp":
        src, dst = args[2], args[3]
        assert src == "-", "only stdin uploads are implemented"
        bucket, name = split_gs(dst)
        data = sys.stdin.buffer.read()
        status, body = http("POST", "/upload/storage/v1/b/%s/o"
                            % urllib.parse.quote(bucket, safe=""),
                            {"uploadType": "media", "name": name}, data)
        sys.stderr.write("Copying from <STDIN>...\n")
        if status != 200:
            sys.stderr.write("ERROR: HTTPError %d\n" % status)
            return 1
        return 0
    if short == "cat":
        bucket, name = split_gs(args[2])
        status, body = http("GET", obj_path(bucket, name), {"alt": "media"})
        if status == 404:
            sys.stderr.write("ERROR: (gcloud.storage.cat) The following "
                             "URLs matched no objects or files:\n%s\n"
                             % args[2])
            return 1
        if status != 200:
            sys.stderr.write("ERROR: HTTPError %d\n" % status)
            return 1
        sys.stdout.buffer.write(body)
        return 0
    if short == "rm":
        bucket, name = split_gs(args[2])
        status, body = http("DELETE", obj_path(bucket, name))
        if status == 404:
            sys.stderr.write("ERROR: (gcloud.storage.rm) The following "
                             "URLs matched no objects or files:\n%s\n"
                             % args[2])
            return 1
        sys.stderr.write("Removing objects:\nRemoving %s...\n" % args[2])
        return 0 if status in (200, 204) else 1
    if short == "ls":
        bucket, name = split_gs(args[2])
        status, body = http("GET", "/storage/v1/b/%s/o"
                            % urllib.parse.quote(bucket, safe=""),
                            {"prefix": name})
        items = json.loads(body.decode()).get("items", [])
        if not items:
            sys.stderr.write(
                "ERROR: (gcloud.storage.ls) One or more URLs matched no "
                "objects.\n")
            return 1
        for it in items:
            print("gs://%s/%s" % (bucket, it["name"]))
        return 0
    if short == "add-metadata":
        instance = args[3]
        key, _, value = flags["metadata"].partition("=")
        status, body = http("POST", "/_test/metadata",
                            {"project": flags.get("project", ""),
                             "instance": instance, "zone": flags["zone"],
                             "key": key, "value": value}, b"", auth=False)
        if status == 412:
            sys.stderr.write(
                "ERROR: (gcloud.compute.instances.add-metadata) Could not "
                "fetch resource:\n - Supplied fingerprint does not match "
                "current metadata fingerprint.\n")
            return 1
        if status != 200:
            sys.stderr.write(
                "ERROR: (gcloud.compute.instances.add-metadata) Could not "
                "fetch resource:\n - The resource 'projects/%s/zones/%s/"
                "instances/%s' was not found\n"
                % (flags.get("project"), flags["zone"], instance))
            return 1
        sys.stderr.write("Updated [https://www.googleapis.com/compute/v1/"
                         "projects/%s/zones/%s/instances/%s].\n"
                         % (flags.get("project"), flags["zone"], instance))
        return 0
    if short == "describe":
        sys.stderr.write("ERROR: (gcloud.compute.instances.describe) "
                         "not found\n")
        return 1
    sys.stderr.write("fake gcloud: unsupported command: %r\n" % (argv,))
    return 2


if __name__ == "__main__":
    sys.exit(main())
