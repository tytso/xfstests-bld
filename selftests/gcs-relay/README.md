# GCS Relay Selftest Suite

Offline end-to-end tests for the `GCE_CURL` hook, `run-fstests/util/gce-gcs-curl`,
and the appliance daemon `test-appliance/files/usr/local/lib/gce-gcs-relay`.

No GCP project, GCS bucket, GCE VM, or network access is required; all servers
bind to loopback ports using an ephemeral self-signed certificate generated
with `openssl` inside a temporary directory.

## Prerequisites

Standard Python 3 (stdlib only) plus: `bash`, `curl`, `jq`, and `openssl`.

## Files

* `test_gcs_relay.py` — test runner exercising the real `run_gce_curl`,
  `gce-ltm-funcs`, `gce-kcs-funcs`, `gce-gcs-curl`, `gce-gcs-relay`, `curl`,
  and `jq`.
* `fake_gcp.py` — in-memory HTTP stand-in for the GCE metadata server (with
  per-project/per-instance attributes, ETags, `wait_for_change` long-polling,
  and optional HTTP 412 fingerprint conflict simulation) and the GCS JSON API.
* `fake_gcloud.py` — stand-in for the `gcloud` CLI subcommands used by
  `gce-gcs-curl` (`storage cp/cat/rm/ls` and `compute instances add-metadata`).
* `fake_ltm.py` — local HTTPS stand-in for the LTM/KCS server using a CN-only
  self-signed certificate (`/login`, `/status`, `/gce-xfstests`, `/slow`, and
  `/delay/<secs>`).

## Running

The suite locates the repository root relative to itself, so it may be invoked
from any working directory:

```sh
./selftests/gcs-relay/test_gcs_relay.py           # full suite (~3–4 min)
./selftests/gcs-relay/test_gcs_relay.py --list    # list test names
./selftests/gcs-relay/test_gcs_relay.py -k relay  # run matching tests
./selftests/gcs-relay/test_gcs_relay.py -v        # show subprocess output
./selftests/gcs-relay/test_gcs_relay.py --keep    # keep tempdir for inspection
```

To test a different checkout without changing directory, pass `--repo PATH`.
The suite removes its temporary directory on success and retains it on failure
for inspection (printing its path on stderr).
