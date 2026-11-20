#!/usr/bin/env python3
"""A fake GCE metadata server and GCS JSON API, for local testing.

One HTTP server (plain HTTP, loopback only) which provides:

  * the parts of the metadata server used by gce-gcs-relay, including
    "hanging GETs" on the instance attributes with ETags;
  * the parts of the GCS JSON API used by gce-gcs-relay (list, download,
    upload, delete, with generation preconditions);
  * a few /_test/ endpoints which let the test driver (and the fake
    gcloud) poke at the state: set metadata, read statistics, age
    objects, inject failures.

All state is kept in memory.
"""

import argparse
import http.server
import json
import socketserver
import sys
import threading
import time
import urllib.parse


class State:
    def __init__(self, args):
        self.cond = threading.Condition()
        self.exempt = threading.local()
        self.bucket = args.bucket
        self.project = args.project
        self.extra_project = args.extra_project
        self.instance = args.instance
        self.zone = args.zone
        self.objects = {}
        self.gen = 1000
        self.stats = {}
        self.token_n = 1
        self.fail = {}          # op -> [count, status]
        self.page_size = args.page_size
        self.no_etag = False
        self.md_delay = 0.0     # how long before a metadata change is seen
        self.md_conflict = False
        self.etag_base = 0
        self.epoch = 0
        self.reset_instances()

    def reset_instances(self):
        self.etag_base += 100
        self.inst_attrs = {
            (self.project, self.instance): {
                "gs_bucket": self.bucket, "gce_xfs_ltm": "YES",
                "ltm_wait": "0"},
            (self.project, "xfstests-kcs"): {
                "gs_bucket": self.bucket, "gce_xfs_kcs": "YES"},
            (self.extra_project, self.instance): {
                "gs_bucket": self.bucket, "gce_xfs_ltm": "YES",
                "ltm_wait": "0"},
        }
        self.inst_etag = {k: self.etag_base + 1 for k in self.inst_attrs}

    def etag(self, key=None):
        return "%016x" % self.inst_etag[key or (self.project, self.instance)]

    def count(self, op):
        self.stats[op] = self.stats.get(op, 0) + 1

    def reset(self):
        self.epoch += 1
        self.reset_instances()
        self.objects = {}
        self.stats = {}
        self.fail = {}
        self.page_size = 0
        self.no_etag = False
        self.md_delay = 0.0
        self.md_conflict = False
        self.cond.notify_all()

    def should_fail(self, op):
        if self.exempt.fake_gcloud:
            return None
        f = self.fail.get(op)
        if f and f[0] > 0:
            f[0] -= 1
            return f[1]
        return None


def rfc3339(t):
    frac = ("%.3f" % (t % 1))[1:]
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(t)) + frac + "Z"


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        pass

    def reply(self, status, body=b"", ctype="application/json", extra=()):
        if isinstance(body, str):
            body = body.encode()
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        for k, v in extra:
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def body(self):
        n = int(self.headers.get("Content-Length") or 0)
        return self.rfile.read(n) if n else b""

    # -- dispatch ---------------------------------------------------

    def handle_any(self):
        st = self.server.state
        # Failures are only injected for the relay daemon, not for the
        # fake gcloud or for the test driver.
        st.exempt.fake_gcloud = bool(self.headers.get("X-Fake-Gcloud"))
        ep = self.headers.get("X-Fake-Epoch")
        if ep is not None:
            with st.cond:
                cur = st.epoch
            if int(ep) != cur:
                self.body()
                return self.reply(404, "{}")
        u = urllib.parse.urlsplit(self.path)
        q = dict(urllib.parse.parse_qsl(u.query, keep_blank_values=True))
        path = u.path
        try:
            if path.startswith("/computeMetadata/v1/"):
                return self.metadata(st, (st.project, st.instance),
                                     path[len("/computeMetadata/v1/"):], q)
            if path.startswith("/computeMetadata/v1."):
                rest = path[len("/computeMetadata/v1."):]
                ident, _, sub = rest.partition("/")
                if "@" in ident:
                    inst, proj = ident.split("@", 1)
                else:
                    inst, proj = ident, st.project
                return self.metadata(st, (proj, inst), sub, q)
            if path.startswith("/_test/"):
                return self.test(st, path[len("/_test/"):], q)
            if path.startswith("/storage/v1/b/") or \
               path.startswith("/upload/storage/v1/b/"):
                return self.storage(st, path, q)
            self.reply(404, "not found\n", "text/plain")
        except BrokenPipeError:
            pass

    do_GET = do_POST = do_DELETE = handle_any

    # -- metadata server --------------------------------------------

    def metadata(self, st, ikey, path, q):
        proj, inst = ikey
        if self.headers.get("Metadata-Flavor") != "Google":
            return self.reply(403, "Missing Metadata-Flavor:Google header.\n",
                              "text/plain")
        with st.cond:
            st.count("md:" + path.split("?")[0])
            status = st.should_fail("md")
        if status:
            return self.reply(status, "injected failure\n", "text/plain")
        if path == "instance/name":
            return self.reply(200, inst, "application/text")
        if path == "project/project-id":
            return self.reply(200, proj, "application/text")
        if path == "instance/service-accounts/default/token":
            with st.cond:
                tok = "tok-%d" % st.token_n
            return self.reply(200, json.dumps({
                "access_token": tok, "expires_in": 3600,
                "token_type": "Bearer"}))
        if path == "instance/attributes/":
            with st.cond:
                if q.get("wait_for_change") == "true" and not st.no_etag:
                    deadline = time.time() + float(q.get("timeout_sec", 3600))
                    last = q.get("last_etag", "NONE")
                    while st.etag(ikey) == last:
                        left = deadline - time.time()
                        if left <= 0:
                            break
                        st.cond.wait(left)
                body = json.dumps(st.inst_attrs[ikey])
                extra = [] if st.no_etag else [("ETag", st.etag(ikey))]
            return self.reply(200, body, "application/json", extra)
        if path.startswith("instance/attributes/"):
            key = path[len("instance/attributes/"):]
            with st.cond:
                val = st.inst_attrs[ikey].get(key)
                etag = st.etag(ikey)
            if val is None:
                return self.reply(404, "not found\n", "text/html")
            return self.reply(200, val, "application/text", [("ETag", etag)])
        if path.startswith("project/attributes/"):
            return self.reply(404, "not found\n", "text/html")
        return self.reply(404, "not found\n", "text/html")

    # -- GCS JSON API -------------------------------------------------

    def storage(self, st, path, q):
        auth = self.headers.get("Authorization", "")
        with st.cond:
            good = "Bearer tok-%d" % st.token_n
        if auth != good:
            with st.cond:
                st.count("gcs:401")
            self.body()
            return self.reply(401, json.dumps({"error": {"code": 401}}))

        upload = path.startswith("/upload/")
        if upload:
            path = path[len("/upload"):]
        parts = path[len("/storage/v1/b/"):].split("/", 2)
        bucket = urllib.parse.unquote(parts[0])
        name = urllib.parse.unquote(parts[2]) if len(parts) > 2 else None
        if len(parts) < 2 or parts[1] != "o":
            return self.reply(404, "{}")

        if upload and self.command == "POST":
            data = self.body()
            with st.cond:
                st.count("gcs:put")
                status = st.should_fail("put")
                if status:
                    return self.reply(status, "{}")
                name = q["name"]
                if q.get("ifGenerationMatch") == "0" and \
                   (bucket, name) in st.objects:
                    return self.reply(412, "{}")
                st.gen += 1
                st.objects[(bucket, name)] = {
                    "data": data, "generation": st.gen,
                    "timeCreated": time.time()}
                st.cond.notify_all()
                return self.reply(200, json.dumps({
                    "name": name, "generation": str(st.gen)}))

        if name is None and self.command == "GET":
            with st.cond:
                st.count("gcs:list")
                status = st.should_fail("list")
                if status:
                    return self.reply(status, "{}")
                prefix = q.get("prefix", "")
                names = sorted(n for (b, n) in st.objects
                               if b == bucket and n.startswith(prefix))
                start = int(q.get("pageToken") or 0)
                doc = {}
                if st.page_size and start + st.page_size < len(names):
                    doc["nextPageToken"] = str(start + st.page_size)
                    names = names[start:start + st.page_size]
                else:
                    names = names[start:]
                items = []
                for n in names:
                    o = st.objects[(bucket, n)]
                    items.append({"name": n,
                                  "generation": str(o["generation"]),
                                  "timeCreated": rfc3339(o["timeCreated"]),
                                  "size": str(len(o["data"]))})
                if items:
                    doc["items"] = items
            return self.reply(200, json.dumps(doc))

        if name is not None and self.command == "GET":
            with st.cond:
                st.count("gcs:get")
                status = st.should_fail("get")
                if status:
                    return self.reply(status, "{}")
                o = st.objects.get((bucket, name))
                if o is None or ("generation" in q and
                                 q["generation"] != str(o["generation"])):
                    return self.reply(404, json.dumps(
                        {"error": {"code": 404}}))
                data = o["data"]
            if q.get("alt") == "media":
                return self.reply(200, data, "application/octet-stream")
            return self.reply(200, json.dumps({"name": name}))

        if name is not None and self.command == "DELETE":
            with st.cond:
                st.count("gcs:delete")
                status = st.should_fail("delete")
                if status:
                    return self.reply(status, "{}")
                o = st.objects.get((bucket, name))
                if o is None:
                    return self.reply(404, json.dumps(
                        {"error": {"code": 404}}))
                if "ifGenerationMatch" in q and \
                   q["ifGenerationMatch"] != str(o["generation"]):
                    return self.reply(412, json.dumps(
                        {"error": {"code": 412}}))
                del st.objects[(bucket, name)]
                st.cond.notify_all()
            return self.reply(204)

        self.reply(400, "{}")

    # -- test control -------------------------------------------------

    def test(self, st, what, q):
        self.body()
        with st.cond:
            if what == "metadata":
                proj = q.get("project") or st.project
                inst = q.get("instance", st.instance)
                zone = q.get("zone", st.zone)
                ikey = (proj, inst)
                if ikey not in st.inst_attrs or zone != st.zone:
                    return self.reply(404, "not found")
                delay = st.md_delay
                conflict = st.md_conflict
                read_etag = st.inst_etag[ikey]
            elif what == "stats":
                return self.reply(200, json.dumps({
                    "stats": st.stats,
                    "attrs": st.inst_attrs[(st.project, st.instance)],
                    "objects": sorted(n for (_, n) in st.objects)}))
            elif what == "reset-stats":
                st.stats = {}
                return self.reply(200, "{}")
            elif what == "reset":
                st.reset()
                return self.reply(200, json.dumps({"epoch": st.epoch}))
            elif what == "age":
                for (b, n), o in st.objects.items():
                    if n == q["name"]:
                        o["timeCreated"] -= float(q["seconds"])
                        return self.reply(200, "{}")
                return self.reply(404, "{}")
            elif what == "fail":
                st.fail[q["op"]] = [int(q.get("count", 1)),
                                    int(q.get("status", 503))]
                return self.reply(200, "{}")
            elif what == "expire-token":
                st.token_n += 1
                return self.reply(200, "{}")
            elif what == "set":
                if "no_etag" in q:
                    st.no_etag = q["no_etag"] == "1"
                if "md_delay" in q:
                    st.md_delay = float(q["md_delay"])
                if "md_conflict" in q:
                    st.md_conflict = q["md_conflict"] == "1"
                if "page_size" in q:
                    st.page_size = int(q["page_size"])
                st.cond.notify_all()
                return self.reply(200, "{}")
            else:
                return self.reply(404, "{}")
        # what == "metadata": apply (possibly after a delay), simulating
        # GCE's fingerprint optimistic lock if md_conflict is enabled.
        if delay:
            time.sleep(delay)
        with st.cond:
            f = st.fail.get("md-set")
            if f and f[0] > 0:
                f[0] -= 1
                st.count("md:412")
                return self.reply(f[1], "fingerprint mismatch")
            if conflict and st.inst_etag[ikey] != read_etag:
                st.count("md:412")
                return self.reply(412, "fingerprint mismatch")
            st.inst_attrs[ikey][q["key"]] = q["value"]
            st.inst_etag[ikey] += 1
            st.count("md-set")
            st.cond.notify_all()
        return self.reply(200, "{}")


class Server(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True
    allow_reuse_address = True
    request_queue_size = 64


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=0)
    ap.add_argument("--portfile")
    ap.add_argument("--project", default="test-proj")
    ap.add_argument("--extra-project", default="example.com:other-proj")
    ap.add_argument("--instance", default="xfstests-ltm")
    ap.add_argument("--zone", default="us-central1-c")
    ap.add_argument("--bucket", default="test-bucket")
    ap.add_argument("--page-size", type=int, default=0)
    args = ap.parse_args()

    srv = Server(("127.0.0.1", args.port), Handler)
    srv.state = State(args)
    if args.portfile:
        with open(args.portfile, "w") as f:
            f.write(str(srv.server_address[1]))
    sys.stderr.write("fake_gcp listening on %d\n" % srv.server_address[1])
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
