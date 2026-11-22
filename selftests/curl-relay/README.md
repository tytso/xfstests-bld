# Curl Relay Selftest Suites

Offline end-to-end tests for the `GCE_CURL` hook, the GCS relay
(`run-fstests/util/gce-gcs-curl` and
`test-appliance/files/usr/local/lib/gce-gcs-relay`), and the on-demand SSH
tunnel wrapper (`run-fstests/util/gce-ssh-curl`).

No GCP project, GCS bucket, GCE VM, or external network access is required; all
servers bind to loopback ports and local Unix-domain sockets using an ephemeral
self-signed certificate generated with `openssl` inside a temporary directory.

## Prerequisites

Standard Python 3 (stdlib only) plus: `bash`, `curl`, `jq`, and `openssl`.

For the `real_ssh` test in `test_ssh_curl.py`, OpenSSH (`ssh`, `ssh-keygen`, and
`sshd`) is also used if installed; otherwise that single test is skipped
automatically.

## Files

* `test_gcs_relay.py` — test runner exercising the real `run_gce_curl`,
  `gce-ltm-funcs`, `gce-kcs-funcs`, `gce-gcs-curl`, `gce-gcs-relay`, `curl`,
  and `jq`.
* `test_ssh_curl.py` — test runner exercising the real `run_gce_curl`,
  `gce-ltm-funcs`, `gce-kcs-funcs`, `gce-ssh-curl` (both the `curl` wrapper and
  its background `--tunnel-daemon`), `curl`, and `jq`.
* `fake_gcp.py` — in-memory HTTP stand-in for the GCE metadata server, the
  GCS JSON API, and the Cloud Pub/Sub REST API (`:pull` and `:acknowledge`),
  fed by simulated GCS `OBJECT_FINALIZE` notifications for objects under
  `gcs-relay/` (with test hooks for delays, duplicates, dropped/malformed
  notifications, missing subscriptions, and OAuth scope/IAM errors).
* `fake_gcloud.py` — stand-in for the `gcloud` CLI subcommands used by
  `gce-gcs-curl` (`storage cp/cat/rm/ls`; explicitly rejects
  `compute instances add-metadata`).
* `fake_ltm.py` — local HTTPS stand-in for the LTM/KCS server using a CN-only
  self-signed certificate (`/login`, `/status`, `/gce-xfstests`, `/slow`, and
  `/delay/<secs>`).
* `fake_ssh.py` — local Unix-domain-socket `-L` port-forwarding stand-in for
  `ssh` used by `test_ssh_curl.py`, with environment-variable knobs for
  connection delays, login failures, and broken-connection disconnects.

## Running

Each suite locates the repository root relative to itself, so it may be invoked
from any working directory:

```sh
./selftests/curl-relay/test_gcs_relay.py           # full GCS relay suite (~3–4 min)
./selftests/curl-relay/test_ssh_curl.py            # full SSH tunnel suite (~1 min)
./selftests/curl-relay/test_ssh_curl.py --list     # list test names
./selftests/curl-relay/test_ssh_curl.py -k ssh     # run matching tests
./selftests/curl-relay/test_ssh_curl.py -v         # show subprocess output
./selftests/curl-relay/test_ssh_curl.py --keep     # keep tempdir for inspection
```

To test a different checkout without changing directory, pass `--repo PATH` to
either script. Each suite removes its temporary directory on success and retains
it on failure for inspection (printing its path on stderr).
