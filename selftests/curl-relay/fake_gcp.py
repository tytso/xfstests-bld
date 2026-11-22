#!/usr/bin/env python3
"""A fake GCE metadata server, GCS JSON API, and Cloud Pub/Sub API, for testing.

One HTTP server (plain HTTP, loopback only) which provides:

  * the parts of the metadata server used by gce-gcs-relay, including
    "hanging GETs" on the instance attributes with ETags;
  * the parts of the GCS JSON API used by gce-gcs-relay (list, download,
    upload, delete, with generation preconditions);
  * the parts of the Cloud Pub/Sub REST API used by gce-gcs-relay
    (:pull and :acknowledge), fed by simulated GCS OBJECT_FINALIZE
    notifications for objects under gcs-relay/;
  * a few /_test/ endpoints which let the test driver (and the fake
    gcloud) poke at the state: set metadata, read statistics, age
    objects, inject failures and Pub/Sub anomalies.

All state is kept in memory.
"""

import argparse
import http.server
import json
import select
import socket
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
        self.pubsub_msg_n = 0
        self.pubsub_ack_n = 0
        self.pubsub_hold = 5.0
        self.pubsub_delay = 0.0
        self.pubsub_dup = 1
        self.pubsub_drop = False
        self.pubsub_fast_empty = False
        self.pubsub_scope_error = False
        self.pubsub_api_disabled = False
        self.pubsub_perm_error = False
        self.reset_instances()
        self.reset_pubsub()

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

    def reset_pubsub(self):
        self.pubsub_subs = {}
        for proj, inst in self.inst_attrs:
            sub_name = "%s-gcs-relay" % inst
            self.pubsub_subs[(proj, sub_name)] = {
                "prefix": "gcs-relay/%s-%s/req/" % (inst, proj),
                "queue": [],
                "ack_deadline": 10.0,
                "deleted": False,
            }

    def enqueue_message(self, sub, attrs, now_wall, data=None, delay=None):
        self.pubsub_msg_n += 1
        if delay is None:
            delay = self.pubsub_delay
        msg = {
            "msg_id": "msg-%d" % self.pubsub_msg_n,
            "ack_id": None,
            "attributes": attrs,
            "publish_time": rfc3339(now_wall),
            "deliver_at": time.monotonic() + delay,
            "leased_until": 0.0,
        }
        if data is not None:
            msg["data"] = data
        sub["queue"].append(msg)
        self.count("pubsub:publish")

    def publish_matching(self, attrs, now_wall):
        obj_id = attrs.get("objectId", "")
        for sub in self.pubsub_subs.values():
            if sub["deleted"]:
                continue
            if obj_id.startswith(sub["prefix"]):
                for _ in range(self.pubsub_dup):
                    self.enqueue_message(sub, dict(attrs), now_wall)

    def etag(self, key=None):
        return "%016x" % self.inst_etag[key or (self.project, self.instance)]

    def count(self, op):
        self.stats[op] = self.stats.get(op, 0) + 1

    def reset(self):
        self.epoch += 1
        self.reset_instances()
        self.reset_pubsub()
        self.objects = {}
        self.stats = {}
        self.fail = {}
        self.page_size = 0
        self.no_etag = False
        self.md_delay = 0.0
        self.md_conflict = False
        self.pubsub_hold = 5.0
        self.pubsub_delay = 0.0
        self.pubsub_dup = 1
        self.pubsub_drop = False
        self.pubsub_fast_empty = False
        self.pubsub_scope_error = False
        self.pubsub_api_disabled = False
        self.pubsub_perm_error = False
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
            if path.startswith("/v1/projects/"):
                return self.pubsub(st, path, q)
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
                now = time.time()
                st.gen += 1
                st.objects[(bucket, name)] = {
                    "data": data, "generation": st.gen,
                    "timeCreated": now}
                if not st.pubsub_drop and name.startswith("gcs-relay/"):
                    attrs = {
                        "bucketId": bucket,
                        "eventTime": rfc3339(now),
                        "eventType": "OBJECT_FINALIZE",
                        "notificationConfig": (
                            "projects/_/buckets/%s/notificationConfigs/1"
                            % bucket),
                        "objectGeneration": str(st.gen),
                        "objectId": name,
                        "payloadFormat": "NONE",
                    }
                    st.publish_matching(attrs, now)
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

    # -- Cloud Pub/Sub REST API ---------------------------------------

    def _pubsub_access_error(self, st):
        if st.pubsub_scope_error:
            return 403, json.dumps({
                "error": {
                    "code": 403,
                    "message": "Request had insufficient authentication "
                               "scopes.",
                    "status": "PERMISSION_DENIED",
                    "details": [{
                        "@type": "type.googleapis.com/google.rpc.ErrorInfo",
                        "reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT",
                        "domain": "googleapis.com",
                    }],
                },
            })
        if st.pubsub_api_disabled:
            return 403, json.dumps({
                "error": {
                    "code": 403,
                    "message": "Cloud Pub/Sub API has not been used in "
                               "project or it is disabled.",
                    "status": "PERMISSION_DENIED",
                    "details": [{
                        "@type": "type.googleapis.com/google.rpc.ErrorInfo",
                        "reason": "SERVICE_DISABLED",
                        "domain": "googleapis.com",
                    }],
                },
            })
        if st.pubsub_perm_error:
            return 403, json.dumps({
                "error": {
                    "code": 403,
                    "message": "User not authorized to perform this action.",
                    "status": "PERMISSION_DENIED",
                },
            })
        return None

    def _client_closed(self):
        try:
            r, _, _ = select.select([self.connection], [], [], 0)
            if r:
                return self.connection.recv(1, socket.MSG_PEEK) == b""
        except OSError:
            return True
        return False

    def pubsub(self, st, path, q):
        raw = self.body()
        auth = self.headers.get("Authorization", "")
        with st.cond:
            good = "Bearer tok-%d" % st.token_n
        if auth != good:
            with st.cond:
                st.count("pubsub:401")
            return self.reply(401, json.dumps({
                "error": {"code": 401, "status": "UNAUTHENTICATED"}}))

        if self.command != "POST":
            return self.reply(405, "{}")

        # Expected: /v1/projects/<project>/subscriptions/<sub_id>:<action>
        rest = path[len("/v1/projects/"):]
        parts = rest.split("/")
        if len(parts) != 3 or parts[1] != "subscriptions" or ":" not in parts[2]:
            return self.reply(404, "{}")
        proj = urllib.parse.unquote(parts[0])
        sub_raw, _, action = parts[2].rpartition(":")
        sub_name = urllib.parse.unquote(sub_raw)
        if action not in ("pull", "acknowledge"):
            return self.reply(404, "{}")

        try:
            req_doc = json.loads(raw.decode("utf-8")) if raw else {}
            if not isinstance(req_doc, dict):
                raise ValueError
        except ValueError:
            return self.reply(400, json.dumps({
                "error": {"code": 400, "message": "malformed JSON"}}))

        with st.cond:
            err = self._pubsub_access_error(st)
            if err:
                return self.reply(err[0], err[1])
            sub = st.pubsub_subs.get((proj, sub_name))
            if sub is None or sub["deleted"]:
                return self.reply(404, json.dumps({
                    "error": {
                        "code": 404,
                        "message": "Resource not found (resource=%s)."
                                   % sub_name,
                        "status": "NOT_FOUND",
                    }}))

            if action == "acknowledge":
                st.count("pubsub:ack")
                status = st.should_fail("pubsub-ack")
                if status:
                    return self.reply(status, "{}")
                ack_ids = set(req_doc.get("ackIds") or [])
                if not ack_ids:
                    return self.reply(400, json.dumps({
                        "error": {"code": 400, "message": "empty ackIds"}}))
                sub["queue"] = [m for m in sub["queue"]
                                if m["ack_id"] not in ack_ids]
                return self.reply(200, "{}")

            # action == "pull"
            st.count("pubsub:pull")
            st.count("pubsub:pull:%s/%s" % (proj, sub_name))
            status = st.should_fail("pubsub-pull")
            if status:
                return self.reply(status, "{}")
            try:
                max_msgs = int(req_doc.get("maxMessages", 10))
                if max_msgs < 1:
                    raise ValueError
            except (TypeError, ValueError):
                return self.reply(400, json.dumps({
                    "error": {"code": 400, "message": "bad maxMessages"}}))

            start_epoch = st.epoch
            hold = 0.0 if (st.pubsub_fast_empty or
                           req_doc.get("returnImmediately")) else st.pubsub_hold
            deadline = time.monotonic() + hold
            while True:
                if st.epoch != start_epoch or self._client_closed():
                    return self.reply(200, "{}")
                err = self._pubsub_access_error(st)
                if err:
                    return self.reply(err[0], err[1])
                if sub["deleted"]:
                    return self.reply(404, json.dumps({
                        "error": {
                            "code": 404,
                            "message": "Resource not found (resource=%s)."
                                       % sub_name,
                            "status": "NOT_FOUND",
                        }}))
                now_mono = time.monotonic()
                ready = [m for m in sub["queue"]
                         if m["deliver_at"] <= now_mono and
                         m["leased_until"] <= now_mono]
                if ready:
                    batch = ready[:max_msgs]
                    received = []
                    for m in batch:
                        st.pubsub_ack_n += 1
                        m["ack_id"] = "ack-%d" % st.pubsub_ack_n
                        m["leased_until"] = now_mono + sub["ack_deadline"]
                        pm = {
                            "attributes": m["attributes"],
                            "messageId": m["msg_id"],
                            "publishTime": m["publish_time"],
                        }
                        if "data" in m:
                            pm["data"] = m["data"]
                        received.append({"ackId": m["ack_id"], "message": pm})
                    return self.reply(200, json.dumps({
                        "receivedMessages": received}))
                left = deadline - now_mono
                if left <= 0:
                    return self.reply(200, "{}")
                # Wake early when a delayed or leased message becomes ready.
                next_ready = None
                for m in sub["queue"]:
                    t = max(m["deliver_at"], m["leased_until"])
                    if t > now_mono and (next_ready is None or t < next_ready):
                        next_ready = t
                wait_for = left
                if next_ready is not None:
                    wait_for = min(wait_for, max(0.01, next_ready - now_mono))
                st.cond.wait(wait_for)

    # -- test control -------------------------------------------------

    def test(self, st, what, q):
        raw = self.body()
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
                    "objects": sorted(n for (_, n) in st.objects),
                    "pubsub_pending": {
                        "%s/%s" % k: len(v["queue"])
                        for k, v in st.pubsub_subs.items()
                    }}))
            elif what == "reset-stats":
                st.stats = {}
                return self.reply(200, "{}")
            elif what == "reset":
                st.reset()
                return self.reply(200, json.dumps({"epoch": st.epoch}))
            elif what == "age":
                secs = float(q["seconds"])
                for (b, n), o in st.objects.items():
                    if n == q["name"]:
                        o["timeCreated"] -= secs
                        for sub in st.pubsub_subs.values():
                            for m in sub["queue"]:
                                if m["attributes"].get("objectId") == n:
                                    m["attributes"]["eventTime"] = rfc3339(
                                        o["timeCreated"])
                        return self.reply(200, "{}")
                return self.reply(404, "{}")
            elif what == "fail":
                st.fail[q["op"]] = [int(q.get("count", 1)),
                                    int(q.get("status", 503))]
                return self.reply(200, "{}")
            elif what == "expire-token":
                st.token_n += 1
                return self.reply(200, "{}")
            elif what == "pubsub-sub":
                proj = q.get("project") or st.project
                sub_name = q.get("subscription") or ("%s-gcs-relay"
                                                     % st.instance)
                sub = st.pubsub_subs.get((proj, sub_name))
                if sub is None:
                    return self.reply(404, "{}")
                if "deleted" in q:
                    sub["deleted"] = q["deleted"] == "1"
                    if sub["deleted"]:
                        sub["queue"] = []
                if "ack_deadline" in q:
                    sub["ack_deadline"] = float(q["ack_deadline"])
                st.cond.notify_all()
                return self.reply(200, "{}")
            elif what == "pubsub-publish":
                proj = q.get("project") or st.project
                sub_name = q.get("subscription") or ("%s-gcs-relay"
                                                     % st.instance)
                sub = st.pubsub_subs.get((proj, sub_name))
                if sub is None or sub["deleted"]:
                    return self.reply(404, "{}")
                doc = json.loads(raw.decode("utf-8")) if raw else {}
                delay = float(q["delay"]) if "delay" in q else 0.0
                st.enqueue_message(sub, doc.get("attributes") or {},
                                   time.time(), data=doc.get("data"),
                                   delay=delay)
                st.cond.notify_all()
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
                if "pubsub_hold" in q:
                    st.pubsub_hold = float(q["pubsub_hold"])
                if "pubsub_delay" in q:
                    st.pubsub_delay = float(q["pubsub_delay"])
                if "pubsub_dup" in q:
                    st.pubsub_dup = int(q["pubsub_dup"])
                if "pubsub_drop" in q:
                    st.pubsub_drop = q["pubsub_drop"] == "1"
                if "pubsub_fast_empty" in q:
                    st.pubsub_fast_empty = q["pubsub_fast_empty"] == "1"
                if "pubsub_scope_error" in q:
                    st.pubsub_scope_error = q["pubsub_scope_error"] == "1"
                if "pubsub_api_disabled" in q:
                    st.pubsub_api_disabled = q["pubsub_api_disabled"] == "1"
                if "pubsub_perm_error" in q:
                    st.pubsub_perm_error = q["pubsub_perm_error"] == "1"
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
