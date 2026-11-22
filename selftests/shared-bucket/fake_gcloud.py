#!/usr/bin/env python3
"""Offline fake 'gcloud' CLI modeling both Flat and HNS GCS buckets.

State lives in a JSON file ($FAKE_GCP_STATE) protected by an exclusive
flock ($FAKE_GCP_LOCK), with one JSONL invocation record appended to
$FAKE_GCLOUD_LOG for every call.

Supported bucket semantics:
  - Flat namespace (hns=False) and Hierarchical Namespace (hns=True):
    * Uploading gs://B/a/b/obj on HNS implicitly creates persistent
      folders 'a/' and 'a/b/' (requiring storage.folders.create when
      VM IAM enforcement is enabled via GCE_CALLER_SA).
    * Deleting the last object in 'a/b/' removes the prefix on Flat
      buckets, while 'a/' and 'a/b/' persist as empty folders on HNS.
    * 'gcloud storage ls' lists objects and (on HNS) persistent folders
      as prefixes (FolderSetting.LIST_AS_PREFIXES): querying an empty
      HNS folder exits 0, while querying an empty Flat prefix exits 1.
    * 'gcloud storage rsync --checksums-only SRC DST' (non-recursive by
      default, FolderSetting.DO_NOT_LIST) exits 1 with 'Did not find
      existing container' when SRC has no objects on BOTH Flat and HNS
      buckets, copies only direct child objects (delimiter='/'), and
      fails on Flat buckets when SRC ends with '/' and a 0-byte 'dir/'
      placeholder object exists alongside child objects.
  - Per-object write counters so tests can verify root files are never
    rewritten during in-place upgrades or secondary-project runs.
  - Fault injection via FAKE_GCLOUD_FAIL_ROOT_CAT,
    FAKE_GCLOUD_FAIL_ROOT_CP_COUNT, and GCE_CALLER_SA IAM checks.
"""

import base64
import fcntl
import fnmatch
import glob
import hashlib
import json
import os
import shutil
import sys
import time


def parse_gs(uri):
    if not uri.startswith("gs://"):
        raise ValueError("not a gs:// URI: %r" % (uri,))
    rest = uri[5:]
    bucket, _, obj = rest.partition("/")
    return bucket, obj


def parent_folders(obj_name):
    """Return ['a/', 'a/b/'] for 'a/b/c' (ignoring trailing slash on folders)."""
    parts = [p for p in obj_name.rstrip("/").split("/") if p]
    out = []
    for i in range(1, len(parts)):
        out.append("/".join(parts[:i]) + "/")
    return out


def md5_b64(data):
    return base64.b64encode(hashlib.md5(data).digest()).decode("ascii")


def load_state(path):
    if not os.path.exists(path) or os.path.getsize(path) == 0:
        return {
            "projects": {},
            "buckets": {},
            "instances": {},
            "firewall_rules": [],
            "pubsub_topics": {},
            "pubsub_subs": {},
            "counters": {},
        }
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def save_state(path, state):
    tmp = path + ".tmp.%d" % os.getpid()
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(state, f, sort_keys=True)
    os.replace(tmp, path)


def check_vm_iam(binfo, caller_sa, caller_proj, perm):
    """Enforce bucket IAM when GCE_CALLER_SA is set and enforce_vm_iam is True."""
    if not caller_sa or not binfo.get("enforce_vm_iam"):
        return True, ""
    same_proj = (binfo.get("project") == caller_proj and bool(caller_proj))
    member = ("serviceAccount:" + caller_sa
              if not caller_sa.startswith("serviceAccount:")
              else caller_sa)
    roles = {r for m, r in binfo.get("iam", []) if m == member}

    # Project-level custom roles (forTestVM / forLTMKCS) grant object permissions
    # inside the same GCP project, but NEVER grant storage.folders.create on HNS
    # and NEVER apply to a bucket owned by a different project.
    granted = set()
    if same_proj:
        granted.update({
            "storage.objects.get",
            "storage.objects.create",
            "storage.objects.delete",
            "storage.objects.list",
        })
    if "roles/storage.legacyBucketWriter" in roles:
        granted.update({
            "storage.buckets.get",
            "storage.objects.create",
            "storage.objects.delete",
            "storage.objects.list",
        })
    if "roles/storage.objectUser" in roles:
        granted.update({
            "storage.objects.get",
            "storage.objects.create",
            "storage.objects.delete",
            "storage.objects.list",
            "storage.folders.create",
            "storage.folders.get",
            "storage.folders.list",
        })

    if perm not in granted:
        return False, (
            "ERROR: HTTPError 403: %s does not have %s access to the "
            "Google Cloud Storage bucket.\n" % (member, perm)
        )
    return True, ""


def put_object(binfo, obj_name, data, caller_sa, caller_proj):
    ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                           "storage.objects.create")
    if not ok:
        return False, err

    if binfo.get("hns"):
        # HNS forbids an object having the exact name of an existing folder.
        if (obj_name.rstrip("/") + "/") in binfo.get("folders", []):
            return False, (
                "ERROR: HTTPError 409: Hierarchical namespace bucket cannot "
                "have an object with the same name as a folder: %s\n"
                % obj_name
            )
        missing_folders = [
            fld for fld in parent_folders(obj_name)
            if fld not in binfo.get("folders", [])
        ]
        if missing_folders:
            ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                                   "storage.folders.create")
            if not ok:
                return False, err
            for fld in missing_folders:
                if fld.rstrip("/") in binfo["objects"]:
                    return False, (
                        "ERROR: HTTPError 409: Hierarchical namespace folder "
                        "collides with object: %s\n" % fld
                    )
                binfo.setdefault("folders", []).append(fld)

    prev = binfo["objects"].get(obj_name)
    writes = (prev["writes"] if prev else 0) + 1
    binfo["objects"][obj_name] = {
        "data_b64": base64.b64encode(data).decode("ascii"),
        "md5": md5_b64(data),
        "writes": writes,
    }
    return True, ""


def parse_metadata_arg(raw):
    """Parse gcloud --metadata (default comma-delimited or '^DELIM^k=v...')."""
    if not raw:
        return {}
    delim = ","
    if raw.startswith("^") and len(raw) >= 3:
        end = raw.find("^", 1)
        if end > 1:
            delim = raw[1:end]
            raw = raw[end + 1:]
    items = {}
    for part in raw.split(delim):
        if not part:
            continue
        if "=" in part:
            k, v = part.split("=", 1)
            items[k] = v
    return items


def strip_global_flags(argv):
    args = []
    flags = {}
    bool_flags = set()
    value_flags = {
        "--account", "--project", "--zone", "--metadata", "--format",
        "--member", "--role", "--condition", "--verbosity", "--file",
        "--description", "--display-name", "--topic", "--topic-project",
        "--event-types", "--object-prefix", "--payload-format",
        "--message-filter", "--ack-deadline", "--message-retention-duration",
        "--expiration-period", "--filter", "--limit", "--machine-type",
        "--network", "--scopes", "--tags", "--image-project",
        "--image-family", "--image", "--provisioning-model",
        "--service-account", "--delete-disks", "--disk",
    }
    i = 0
    while i < len(argv):
        a = argv[i]
        if a in value_flags and i + 1 < len(argv):
            flags.setdefault(a[2:], []).append(argv[i + 1])
            i += 2
            continue
        if a.startswith("--") and "=" in a:
            k, v = a[2:].split("=", 1)
            flags.setdefault(k, []).append(v)
            i += 1
            continue
        if a in ("-q", "--quiet", "--raw", "--no-standard-images",
                 "--brief", "--checksums-only", "--recursive", "-r", "-b",
                 "--no-auto-delete"):
            bool_flags.add(a)
            i += 1
            continue
        args.append(a)
        i += 1
    return args, flags, bool_flags


def last_flag(flags, key, default=""):
    vals = flags.get(key)
    return vals[-1] if vals else default


def handle_storage(state, args, flags, bool_flags, caller_sa, caller_proj):
    sub = args[1] if len(args) > 1 else ""

    if sub == "buckets":
        action = args[2] if len(args) > 2 else ""
        if action == "describe":
            bname, _ = parse_gs(args[3])
            binfo = state["buckets"].get(bname)
            if not binfo:
                sys.stderr.write("ERROR: bucket not found: %s\n" % bname)
                return 1
            pinfo = state["projects"].get(binfo["project"], {})
            pnum = str(pinfo.get("number", "100001"))
            fmt = last_flag(flags, "format")
            if "projectNumber" in fmt:
                print(pnum)
            else:
                print(json.dumps({"name": bname, "projectNumber": pnum}))
            return 0
        if action == "add-iam-policy-binding":
            bname, _ = parse_gs(args[3])
            binfo = state["buckets"].get(bname)
            if not binfo:
                sys.stderr.write("ERROR: bucket not found: %s\n" % bname)
                return 1
            member = last_flag(flags, "member")
            role = last_flag(flags, "role")
            pair = [member, role]
            if pair not in binfo.setdefault("iam", []):
                binfo["iam"].append(pair)
            print("Updated IAM policy for bucket [%s]." % bname)
            return 0
        if action == "notifications":
            nact = args[3] if len(args) > 3 else ""
            if nact == "list":
                bname, _ = parse_gs(args[4])
                binfo = state["buckets"].get(bname, {})
                print(json.dumps(binfo.get("notifications", [])))
                return 0
            if nact == "create":
                bname, _ = parse_gs(args[4])
                binfo = state["buckets"][bname]
                topic = last_flag(flags, "topic")
                prefix = last_flag(flags, "object-prefix")
                tname = topic.split("/")[-1]
                tproj = topic.split("/")[1] if "/" in topic else caller_proj
                state["pubsub_topics"].setdefault(tproj, set())
                if isinstance(state["pubsub_topics"][tproj], list):
                    if tname not in state["pubsub_topics"][tproj]:
                        state["pubsub_topics"][tproj].append(tname)
                else:
                    state["pubsub_topics"][tproj] = [tname]
                nid = str(len(binfo.setdefault("notifications", [])) + 1)
                binfo["notifications"].append({
                    "Notification Configuration": {
                        "id": nid,
                        "topic": "//pubsub.googleapis.com/" + topic,
                        "object_name_prefix": prefix,
                        "event_types": ["OBJECT_FINALIZE"],
                    }
                })
                return 0
            if nact == "delete":
                return 0

    if sub == "objects":
        act = args[2] if len(args) > 2 else ""
        if act == "describe":
            bname, oname = parse_gs(args[3])
            binfo = state["buckets"].get(bname)
            if not binfo or oname not in binfo["objects"]:
                sys.stderr.write(
                    "ERROR: (gcloud.storage.objects.describe) URL matched no "
                    "objects: %s\n" % args[3]
                )
                return 1
            ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                                   "storage.objects.get")
            if not ok:
                sys.stderr.write(err)
                return 1
            print("name: %s\ncustom_fields:\n  arch: amd64\n  ver: 6.12.0\n"
                  % oname)
            return 0

    if sub == "hash":
        target = args[2]
        if target.startswith("gs://"):
            bname, oname = parse_gs(target)
            binfo = state["buckets"].get(bname)
            if not binfo or oname not in binfo["objects"]:
                sys.stderr.write(
                    "ERROR: (gcloud.storage.hash) URL matched no objects: %s\n"
                    % target
                )
                return 1
            ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                                   "storage.objects.get")
            if not ok:
                sys.stderr.write(err)
                return 1
            print("md5_hash: %s" % binfo["objects"][oname]["md5"])
            return 0
        if not os.path.isfile(target):
            sys.stderr.write("ERROR: file not found: %s\n" % target)
            return 1
        with open(target, "rb") as f:
            print("md5_hash: %s" % md5_b64(f.read()))
        return 0

    if sub == "cat":
        uri = args[2]
        bname, oname = parse_gs(uri)
        if (oname == "gce_xfstests.config" and
                os.environ.get("FAKE_GCLOUD_FAIL_ROOT_CAT") == "1"):
            sys.stderr.write(
                "ERROR: (gcloud.storage.cat) HTTPError 503: backend error\n"
            )
            return 1
        binfo = state["buckets"].get(bname)
        if not binfo:
            sys.stderr.write("ERROR: bucket not found: %s\n" % bname)
            return 1
        ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                               "storage.objects.get")
        if not ok:
            sys.stderr.write(err)
            return 1
        obj = binfo["objects"].get(oname)
        if not obj:
            sys.stderr.write(
                "ERROR: (gcloud.storage.cat) The following URLs matched no "
                "objects or files:\n%s\n" % uri
            )
            return 1
        sys.stdout.buffer.write(base64.b64decode(obj["data_b64"]))
        return 0

    if sub == "cp":
        srcs = args[2:-1]
        dst = args[-1]
        dst_is_gs = dst.startswith("gs://")
        dst_trailing_slash = dst.endswith("/")

        if dst_is_gs:
            dbucket, dobj = parse_gs(dst)
            binfo = state["buckets"].get(dbucket)
            if not binfo:
                sys.stderr.write("ERROR: bucket not found: %s\n" % dbucket)
                return 1
            expanded_srcs = []
            for src in srcs:
                if (src != "-" and not src.startswith("gs://") and
                        any(ch in src for ch in "*?[")):
                    matches = sorted(glob.glob(src))
                    if not matches:
                        sys.stderr.write("ERROR: file not found: %s\n" % src)
                        return 1
                    expanded_srcs.extend(matches)
                else:
                    expanded_srcs.append(src)
            for src in expanded_srcs:
                if src == "-":
                    data = sys.stdin.buffer.read()
                    target_name = dobj
                elif src.startswith("gs://"):
                    sbucket, sobj = parse_gs(src)
                    sbinfo = state["buckets"].get(sbucket)
                    if not sbinfo or sobj not in sbinfo["objects"]:
                        sys.stderr.write(
                            "ERROR: (gcloud.storage.cp) URL matched no "
                            "objects: %s\n" % src
                        )
                        return 1
                    ok, err = check_vm_iam(sbinfo, caller_sa, caller_proj,
                                           "storage.objects.get")
                    if not ok:
                        sys.stderr.write(err)
                        return 1
                    data = base64.b64decode(sbinfo["objects"][sobj]["data_b64"])
                    base = os.path.basename(sobj.rstrip("/"))
                    target_name = (
                        (dobj.rstrip("/") + "/" + base).lstrip("/")
                        if (dst_trailing_slash or len(srcs) > 1 or dobj == "")
                        else dobj
                    )
                else:
                    if not os.path.isfile(src):
                        sys.stderr.write("ERROR: file not found: %s\n" % src)
                        return 1
                    with open(src, "rb") as f:
                        data = f.read()
                    base = os.path.basename(src)
                    target_name = (
                        (dobj.rstrip("/") + "/" + base).lstrip("/")
                        if (dst_trailing_slash or len(srcs) > 1 or dobj == "")
                        else dobj
                    )
                ok, err = put_object(binfo, target_name, data, caller_sa,
                                     caller_proj)
                if not ok:
                    sys.stderr.write(err)
                    return 1
            return 0
        else:
            # Copy gs:// -> local
            assert len(srcs) == 1 and srcs[0].startswith("gs://"), srcs
            sbucket, sobj = parse_gs(srcs[0])
            fail_key = "fail_root_cp"
            rem_env = os.environ.get("FAKE_GCLOUD_FAIL_ROOT_CP_COUNT")
            if sobj == "gce_xfstests.config" and rem_env is not None:
                rem = state["counters"].get(fail_key, int(rem_env))
                if rem > 0:
                    state["counters"][fail_key] = rem - 1
                    sys.stderr.write(
                        "ERROR: (gcloud.storage.cp) HTTPError 503: "
                        "transient backend error\n"
                    )
                    return 1
            binfo = state["buckets"].get(sbucket)
            if not binfo:
                sys.stderr.write("ERROR: bucket not found: %s\n" % sbucket)
                return 1
            ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                                   "storage.objects.get")
            if not ok:
                sys.stderr.write(err)
                return 1
            if sobj not in binfo["objects"]:
                sys.stderr.write(
                    "ERROR: (gcloud.storage.cp) The following URLs matched "
                    "no objects or files:\n%s\n" % srcs[0]
                )
                return 1
            data = base64.b64decode(binfo["objects"][sobj]["data_b64"])
            if dst.endswith("/") or os.path.isdir(dst):
                out_path = os.path.join(dst, os.path.basename(sobj))
            else:
                os.makedirs(os.path.dirname(os.path.abspath(dst)),
                            exist_ok=True)
                out_path = dst
            with open(out_path, "wb") as f:
                f.write(data)
            return 0

    if sub == "rm":
        matched_any = False
        for uri in args[2:]:
            bname, oname = parse_gs(uri)
            binfo = state["buckets"].get(bname)
            if not binfo:
                continue
            ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                                   "storage.objects.delete")
            if not ok:
                sys.stderr.write(err)
                return 1
            if oname in binfo["objects"]:
                del binfo["objects"][oname]
                # Note: on HNS buckets, parent folders intentionally remain!
                matched_any = True
        if not matched_any:
            sys.stderr.write(
                "ERROR: (gcloud.storage.rm) The following URLs matched no "
                "objects or files:\n%s\n" % " ".join(args[2:])
            )
            return 1
        return 0

    if sub == "mv":
        src_uri, dst_uri = args[2], args[3]
        sbucket, spat = parse_gs(src_uri)
        dbucket, dprefix = parse_gs(dst_uri)
        sbinfo = state["buckets"][sbucket]
        dbinfo = state["buckets"][dbucket]
        # Match direct objects when pattern ends with '/*' (non-** glob)
        matched = []
        for k in sorted(sbinfo["objects"]):
            if k.endswith("/"):
                continue
            if "**" not in spat and "/" in spat:
                parent = spat.rsplit("/", 1)[0]
                if "/" in k[len(parent) + 1:] if k.startswith(parent + "/") else True:
                    if not fnmatch.fnmatchcase(k, spat):
                        continue
            if fnmatch.fnmatchcase(k, spat) and (
                "**" in spat or k.count("/") == spat.count("/")
            ):
                matched.append(k)
        if not matched:
            sys.stderr.write(
                "ERROR: (gcloud.storage.mv) URL matched no objects: %s\n"
                % src_uri
            )
            return 1
        for k in matched:
            data = base64.b64decode(sbinfo["objects"][k]["data_b64"])
            target = (
                (dprefix.rstrip("/") + "/" + os.path.basename(k)).lstrip("/")
                if dst_uri.endswith("/")
                else dprefix
            )
            ok, err = put_object(dbinfo, target, data, caller_sa, caller_proj)
            if not ok:
                sys.stderr.write(err)
                return 1
            del sbinfo["objects"][k]
        return 0

    if sub == "ls":
        uri = args[2] if len(args) > 2 else ""
        bname, pat = parse_gs(uri)
        binfo = state["buckets"].get(bname)
        if not binfo:
            sys.stderr.write(
                "ERROR: (gcloud.storage.ls) gs://%s not found.\n" % bname
            )
            return 1
        if "-b" in bool_flags or pat == "":
            print("gs://%s/" % bname)
            return 0
        ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                               "storage.objects.list")
        if not ok:
            sys.stderr.write(err)
            return 1

        objs = binfo["objects"]
        folders = set(binfo.get("folders", [])) if binfo.get("hns") else set()

        # Wildcard query (e.g. **results* or dir/*)
        if any(ch in pat for ch in "*?[]"):
            lines = []
            for k in sorted(objs):
                if fnmatch.fnmatchcase(k, pat):
                    lines.append("gs://%s/%s" % (bname, k))
            if not lines:
                sys.stderr.write(
                    "ERROR: (gcloud.storage.ls) One or more URLs matched no "
                    "objects.\n"
                )
                return 1
            for ln in lines:
                print(ln)
            return 0

        # Exact object match (when no trailing slash)
        if not pat.endswith("/") and pat in objs:
            print("gs://%s/%s" % (bname, pat))
            return 0

        # Directory / prefix listing (FolderSetting.LIST_AS_PREFIXES)
        prefix = pat.rstrip("/") + "/"
        entries = set()
        has_container = prefix in folders
        for k in objs:
            if k == prefix:
                # 0-byte Console placeholder object at 'dir/'
                entries.add("gs://%s/%s" % (bname, prefix))
                has_container = True
            elif k.startswith(prefix):
                has_container = True
                rem = k[len(prefix):]
                if "/" in rem:
                    sub_dir = rem.split("/", 1)[0] + "/"
                    entries.add("gs://%s/%s%s" % (bname, prefix, sub_dir))
                else:
                    entries.add("gs://%s/%s" % (bname, k))
        for fld in folders:
            if fld.startswith(prefix) and fld != prefix:
                has_container = True
                rem = fld[len(prefix):].rstrip("/")
                if "/" not in rem and rem:
                    entries.add("gs://%s/%s%s/" % (bname, prefix, rem))

        if not entries and not has_container:
            sys.stderr.write(
                "ERROR: (gcloud.storage.ls) One or more URLs matched no "
                "objects.\n"
            )
            return 1
        for ln in sorted(entries):
            print(ln)
        return 0

    if sub == "rsync":
        src_uri, dst_dir = args[2], args[3]
        recursive = ("--recursive" in bool_flags or "-r" in bool_flags)
        bname, prefix_raw = parse_gs(src_uri)
        binfo = state["buckets"].get(bname)
        if not binfo:
            sys.stderr.write(
                "ERROR: (gcloud.storage.rsync) Did not find existing "
                "container at: %s\n" % src_uri
            )
            return 1
        ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                               "storage.objects.list")
        if not ok:
            sys.stderr.write(err)
            return 1

        objs = binfo["objects"]
        prefix = prefix_raw.rstrip("/") + "/"

        # Flat-bucket quirk: if SRC has a trailing slash AND both a 0-byte
        # 'dir/' placeholder object and child objects exist, gcloud rsync
        # errors with 'matched more than one URL'.
        if (not binfo.get("hns") and prefix_raw.endswith("/") and
                prefix in objs and
                any(k.startswith(prefix) and k != prefix for k in objs)):
            sys.stderr.write(
                "ERROR: (gcloud.storage.rsync) %s matched more than one URL.\n"
                % src_uri
            )
            return 1

        # rsync uses FolderSetting.DO_NOT_LIST: empty HNS folders do NOT count
        # as an existing container when no objects exist under prefix!
        any_obj_under_prefix = any(
            k.startswith(prefix) and k != prefix for k in objs
        )
        if not any_obj_under_prefix:
            sys.stderr.write(
                "ERROR: (gcloud.storage.rsync) Did not find existing "
                "container at: %s\n" % src_uri
            )
            return 1

        os.makedirs(dst_dir, exist_ok=True)
        for k in sorted(objs):
            if not k.startswith(prefix) or k == prefix or k.endswith("/"):
                continue
            rel = k[len(prefix):]
            if not recursive and "/" in rel:
                continue
            ok, err = check_vm_iam(binfo, caller_sa, caller_proj,
                                   "storage.objects.get")
            if not ok:
                sys.stderr.write(err)
                return 1
            out_path = os.path.join(dst_dir, rel)
            os.makedirs(os.path.dirname(out_path), exist_ok=True)
            data = base64.b64decode(objs[k]["data_b64"])
            with open(out_path, "wb") as f:
                f.write(data)
        return 0

    sys.stderr.write("fake_gcloud: unsupported storage subcommand: %r\n"
                     % (args,))
    return 2


def main():
    state_path = os.environ["FAKE_GCP_STATE"]
    lock_path = os.environ["FAKE_GCP_LOCK"]
    log_path = os.environ.get("FAKE_GCLOUD_LOG", "")
    caller_sa = os.environ.get("GCE_CALLER_SA", "")

    argv = sys.argv[1:]
    with open(lock_path, "a+", encoding="utf-8") as lock_fh:
        fcntl.flock(lock_fh.fileno(), fcntl.LOCK_EX)
        if log_path:
            with open(log_path, "a", encoding="utf-8") as lf:
                lf.write(json.dumps({
                    "time": time.time(),
                    "argv": argv,
                    "caller_sa": caller_sa,
                }) + "\n")

        args, flags, bool_flags = strip_global_flags(argv)
        proj = last_flag(flags, "project", os.environ.get("GCE_PROJECT", ""))
        state = load_state(state_path)

        rc = dispatch(state, args, flags, bool_flags, caller_sa, proj)
        save_state(state_path, state)
        return rc


def dispatch(state, args, flags, bool_flags, caller_sa, proj):
    if not args:
        return 0
    top = args[0]

    if top == "storage":
        return handle_storage(state, args, flags, bool_flags, caller_sa, proj)

    if top == "auth":
        if args[1:2] == ["list"]:
            print("tester@example.com")
        return 0

    if top == "config":
        if args[1:2] == ["list"]:
            print("tester@example.com")
        return 0

    if top == "services":
        return 0

    if top == "projects":
        sub = args[1] if len(args) > 1 else ""
        if sub == "describe":
            target = args[2]
            # Lookup by project ID or project number
            pinfo = state["projects"].get(target)
            pid = target
            if not pinfo:
                for k, v in state["projects"].items():
                    if str(v.get("number")) == str(target):
                        pinfo = v
                        pid = k
                        break
            if not pinfo:
                # Auto-register standard image project xfstests-cloud
                if target == "xfstests-cloud":
                    pinfo = {"number": "999999"}
                else:
                    sys.stderr.write("ERROR: project not found: %s\n" % target)
                    return 1
            fmt = last_flag(flags, "format")
            if "projectNumber" in fmt:
                print(pinfo["number"])
            elif "projectId" in fmt:
                print(pid)
            else:
                print("projectId: %s\nprojectNumber: '%s'"
                      % (pid, pinfo["number"]))
            return 0
        if sub == "add-iam-policy-binding":
            return 0

    if top == "iam":
        sub = args[1] if len(args) > 1 else ""
        if sub == "roles":
            act = args[2] if len(args) > 2 else ""
            pinfo = state["projects"].setdefault(proj, {"number": "100001"})
            roles = pinfo.setdefault("roles", [])
            rname = args[3] if len(args) > 3 else ""
            if act == "describe":
                return 0 if rname in roles else 1
            if act in ("create", "undelete", "update"):
                if rname and rname not in roles:
                    roles.append(rname)
                return 0
        if sub == "service-accounts":
            act = args[2] if len(args) > 2 else ""
            pinfo = state["projects"].setdefault(proj, {"number": "100001"})
            sas = pinfo.setdefault("service_accounts", [])
            if act == "describe":
                sa = args[3]
                return 0 if sa in sas else 1
            if act == "create":
                name = args[3]
                pdom = proj.replace(":", ".")
                if "." in proj:
                    # domain-scoped project: name@proj.iam.gserviceaccount.com
                    dom, _, short = proj.partition(":")
                    sa = "%s@%s.%s.iam.gserviceaccount.com" % (name, short, dom)
                else:
                    sa = "%s@%s.iam.gserviceaccount.com" % (name, pdom)
                if sa not in sas:
                    sas.append(sa)
                # Also store the exact email form gce-xfstests constructs
                alt = "%s@%s.iam.gserviceaccount.com" % (name, proj)
                if alt not in sas:
                    sas.append(alt)
                return 0
            if act == "list":
                for sa in sas:
                    print(sa)
                return 0

    if top == "pubsub":
        sub = args[1] if len(args) > 1 else ""
        act = args[2] if len(args) > 2 else ""
        if sub == "topics":
            topics = state["pubsub_topics"].setdefault(proj, [])
            tname = args[3] if len(args) > 3 else ""
            if act == "describe":
                if tname in topics:
                    print(json.dumps({"name": "projects/%s/topics/%s"
                                              % (proj, tname)}))
                    return 0
                sys.stderr.write("ERROR: NOT_FOUND: topic %s\n" % tname)
                return 1
            if act == "list":
                return 0
        if sub == "subscriptions":
            subs = state["pubsub_subs"].setdefault(proj, {})
            sname = args[3] if len(args) > 3 else ""
            if act == "describe":
                if sname not in subs:
                    sys.stderr.write("ERROR: NOT_FOUND: sub %s\n" % sname)
                    return 1
                print(json.dumps(subs[sname]))
                return 0
            if act == "create":
                tproj = last_flag(flags, "topic-project", proj)
                tname = last_flag(flags, "topic")
                subs[sname] = {
                    "topic": "projects/%s/topics/%s" % (tproj, tname),
                    "filter": last_flag(flags, "message-filter"),
                }
                return 0
            if act in ("delete", "add-iam-policy-binding"):
                if act == "delete":
                    subs.pop(sname, None)
                return 0

    if top == "compute":
        sub = args[1] if len(args) > 1 else ""
        act = args[2] if len(args) > 2 else ""
        if sub == "project-info":
            return 0
        if sub in ("zones", "networks"):
            if act == "describe":
                return 0
            if act == "list":
                print("us-central1-c")
                return 0
        if sub == "images":
            if act in ("describe-from-family", "describe", "list"):
                print("name: xfstests-20261122")
                return 0
        if sub == "machine-types":
            if act == "list":
                print("NAME ZONE CPUS MEMORY_GB")
                print("e2-standard-2 us-central1-c 2 8.00")
                print("n2-standard-2 us-central1-c 2 8.00")
                return 0
        if sub == "firewall-rules":
            if act == "list":
                rname = args[3] if len(args) > 3 else ""
                print("NAME NETWORK DIRECTION")
                if rname in state["firewall_rules"]:
                    print("%s default INGRESS" % rname)
                return 0
            if act == "create":
                rname = args[3]
                if rname not in state["firewall_rules"]:
                    state["firewall_rules"].append(rname)
                return 0
        if sub == "instances":
            p_insts = state["instances"].setdefault(proj, {})
            if act == "create":
                iname = args[3]
                md = parse_metadata_arg(last_flag(flags, "metadata"))
                p_insts[iname] = {
                    "name": iname,
                    "project": proj,
                    "zone": last_flag(flags, "zone", "us-central1-c"),
                    "service_account": last_flag(flags, "service-account"),
                    "metadata": md,
                    "status": "RUNNING",
                    "natIP": "127.0.0.1",
                    "networkIP": "10.128.0.2",
                }
                print("Created [%s]." % iname)
                return 0
            if act == "describe":
                iname = args[3]
                info = p_insts.get(iname)
                if not info or info.get("deleted"):
                    sys.stderr.write(
                        "ERROR: (gcloud.compute.instances.describe) "
                        "Instance %s not found\n" % iname
                    )
                    return 1
                fmt = last_flag(flags, "format")
                if "natIP" in fmt:
                    print(info["natIP"])
                elif "networkIP" in fmt:
                    print(info["networkIP"])
                else:
                    print(json.dumps(info))
                return 0
            if act == "add-metadata":
                iname = args[3]
                md = parse_metadata_arg(last_flag(flags, "metadata"))
                info = p_insts.setdefault(iname, {
                    "name": iname,
                    "project": proj,
                    "zone": last_flag(flags, "zone", "us-central1-c"),
                    "metadata": {},
                    "status": "RUNNING",
                    "natIP": "127.0.0.1",
                    "networkIP": "10.128.0.2",
                })
                info.setdefault("metadata", {}).update(md)
                return 0
            if act == "delete":
                for iname in args[3:]:
                    if iname in p_insts:
                        p_insts[iname]["deleted"] = True
                return 0
            if act == "set-disk-auto-delete":
                return 0
            if act == "list":
                for iname, info in sorted(p_insts.items()):
                    if not info.get("deleted"):
                        print("%s %s" % (iname, info["zone"]))
                return 0

    sys.stderr.write("fake_gcloud: unsupported command: %r\n" % (args,))
    return 2


if __name__ == "__main__":
    sys.exit(main())
