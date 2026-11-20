# LTM and KCS Server Developer Notes

These notes are for debugging the LTM and KCS servers.

## Overview

The LTM and KCS go source code is located at [gce-server/](../test-appliance/files/usr/local/lib/gce-server) in this repo. When you use the default GCE test appliance VM image or build your own image, the source code is located at `/usr/local/lib/gce-server/`, and pre-compiled into binary file `ltm` and `kcs` at `/usr/local/lib/bin/`. They are executed when LTM or KCS server is launched respectively.

## SSH into LTM or KCS server to check logs

When something unexpected happens, you can ssh into the LTM or KCS server with command:

        gce-xfstests ssh [xfstests-ltm|xfstests-kcs]

Where `xfstests-ltm` is for LTM server and `xfstests-kcs` is for KCS server.

The log files are located at `/var/log/go/` on the server. The web server's log goes to `server.log`, while logs for each request goes to separate folders under `ltm_logs/` or `kcs_logs/`, named with testID.

## Cache PD for KCS server

KCS server uses a persistent disk to cache data in order to speed up building. This cache pd is mounted to `/cache/` on the KCS server:

* `/cache/repositories/`: cached git repos from previous build tasks.
* `/cache/log`: packed log files from previous KCS runs. KCS server shuts down itself when stays idle, and all the log files during this run are packed in a tarball here, named with the shutdown time.
* `/cache/ccache/`: caches for ccache.

## Run Server in Debug Mode

If you make changes to the go source code, the normal approach for testing is to build a new image and launch new servers with it. You can also change code on the server directly and run it in debug mode.

After you ssh onto the server, you need to stop the running process that registered as system service first, and navigate to the go source code where you can run the server manaully.

On the LTM server:

        systemctl disable gce-ltm.service
        systemctl stop gce-ltm.service
        cd /usr/local/lib/gce-server/ltm
        go run .

On the KCS server:

        systemctl disable gce-kcs.service
        systemctl stop gce-kcs.service
        cd /usr/local/lib/gce-server/kcs
        go run .

To run the server in debug mode, set `DEBUG = true` in [logging.go](../test-appliance/files/usr/local/lib/gce-server/util/logging/logging.go) at `/usr/local/lib/gce-server/util/logging/logging.go` before you execute these commands.

In debug mode, logs are redirected to the console with human-friendly format, and KCS server will not shut down itself automatically.

Check code docs and function comments for more details about how the server works.

## The GCS relay

When gce-xfstests is configured with `GCE_CURL=gce-gcs-curl` (see
[Using the GCS relay to reach the LTM and KCS
servers](gce-xfstests.md#using-the-gcs-relay-to-reach-the-ltm-and-kcs-servers)),
requests do not arrive at the server directly from the user's
machine.  Instead the [gce-gcs-curl](../run-fstests/util/gce-gcs-curl)
script uploads each request to the GCS bucket, and the
[gce-gcs-relay](../test-appliance/files/usr/local/lib/gce-gcs-relay)
daemon, which runs on the LTM and KCS VMs as `gce-gcs-relay.service`,
fetches the request and sends it to `https://127.0.0.1:443`.  From the
point of view of the LTM or KCS server, the request comes from the
local host.  The comments at the beginning of the gce-gcs-relay script
describe the protocol.

The relay daemon logs one line for each request (but never the
contents of a request or response) to the journal:

        journalctl -u gce-gcs-relay.service

To get more details about what it is doing, it can be run by hand:

        systemctl stop gce-gcs-relay.service
        /usr/local/lib/gce-gcs-relay --debug

While a request is in transit it can be found in the GCS bucket as
`gs://$GS_BUCKET/gcs-relay/<instance>-$GCE_PROJECT/req/<id>.json`, and
its response as
`gs://$GS_BUCKET/gcs-relay/<instance>-$GCE_PROJECT/rsp/<id>.json`.

### Instance separation and concurrency

Although `gce-ltm-funcs` and `gce-kcs-funcs` use the same URL hostname
(`a.$GCE_PROJECT.gce-xfstests`) to match the wildcard certificate,
`run_gce_curl` (in `run-fstests/util/get-config`) passes the target VM
name (`xfstests-ltm` or `xfstests-kcs`) via `GCE_INSTANCE`, along with
`GCE_PROJECT`.  `gce-gcs-curl` uses both to scope objects under
`gcs-relay/<instance>-$GCE_PROJECT/` and updates `gcs_relay` only on
that VM's instance metadata.  On the appliance side, each VM's
`gce-gcs-relay` queries `instance/name` and `project/project-id` from
the GCE metadata server at startup, watches only its own instance
attributes, and lists only its own `gcs-relay/<instance>-$GCE_PROJECT/`
prefix.  This keeps LTM and KCS traffic separate (waking one VM does
not wake the other) and also allows multiple GCE projects using
gce-xfstests to share a single GCS bucket without colliding on
`xfstests-ltm` or `xfstests-kcs`.  Internal RPCs between LTM and
KCS (`/internal` and `/internal-status`) do not go through the relay;
the Go servers talk directly to each other over their VPC internal IPs.

Parallel client invocations against the same VM are also supported:

* Each request carries a unique `<id>` (`<UTC timestamp>-<pid>-<64-bit
  random hex>`), giving every in-flight call its own `req/<id>.json`
  and `rsp/<id>.json` objects.  Cookie jars written by `gce-gcs-curl`
  are replaced atomically via a temporary file.
* `gce-gcs-relay` keeps its metadata long-poll and bucket scan on the
  main thread and dispatches requests across a worker thread pool
  (`--max-workers`, default 8), tracking in-flight `(name, generation)`
  pairs and claiming each object with an `ifGenerationMatch` delete
  before forwarding it to `https://127.0.0.1:443`.  A slow endpoint
  therefore does not head-of-line block other concurrent requests, and
  `SIGTERM` waits for already-claimed requests to finish while leaving
  any unclaimed ones in `req/` for the next run.
* `gcs_relay` is only a wakeup signal, not a single-slot mailbox; a
  single wakeup causes the daemon to list and claim all pending objects
  under `req/` and continue burst-scanning.  If simultaneous
  `gcloud compute instances add-metadata` calls hit a GCE metadata
  fingerprint conflict (HTTP 412), `gce-gcs-curl` retries with jittered
  backoff, and before failing on a persistent signal error it checks
  whether `req/<id>.json` has already been claimed by a relay woken by
  another client.

### Testing the relay locally

An offline loopback self-test suite is located in
[selftests/gcs-relay/](../selftests/gcs-relay/README.md) and exercises
`gce-gcs-curl`, `gce-gcs-relay`, and the `run_gce_curl` shell hooks
without touching real GCP or GCS resources:

        ./selftests/gcs-relay/test_gcs_relay.py
