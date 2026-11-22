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

### Instance separation, Pub/Sub notifications, and concurrency

Although `gce-ltm-funcs` and `gce-kcs-funcs` use the same URL hostname
(`a.$GCE_PROJECT.gce-xfstests`) to match the wildcard certificate,
`run_gce_curl` (in `run-fstests/util/get-config`) passes the target VM
name (`xfstests-ltm` or `xfstests-kcs`) via `GCE_INSTANCE`, along with
`GCE_PROJECT`.  `gce-gcs-curl` uses both to scope objects under
`gcs-relay/<instance>-$GCE_PROJECT/` and uploads the request directly
without touching GCE instance metadata.

`gce-xfstests setup` configures a Cloud Storage `OBJECT_FINALIZE`
notification on `gs://$GS_BUCKET` (`--object-prefix=gcs-relay/`,
`--payload-format=none`) publishing to the `gce-xfstests-gcs-relay`
Pub/Sub topic (in `$GCE_PROJECT` when `$GS_BUCKET` belongs to
`$GCE_PROJECT`, or in the bucket's owning project when `$GS_BUCKET`
lives in another project or organization), and creates one filtered pull
subscription per server VM in `$GCE_PROJECT` (`xfstests-ltm-gcs-relay`
and `xfstests-kcs-gcs-relay`) with filter:

        hasPrefix(attributes.objectId, "gcs-relay/<instance>-<project>/req/")

On the appliance side, each VM's `gce-gcs-relay` queries
`instance/name` and `project/project-id` from the GCE metadata server
at startup, long-polls its own `<instance>-gcs-relay` subscription over
the Pub/Sub REST API, and only reads or lists objects under its own
`gcs-relay/<instance>-$GCE_PROJECT/` prefix.  This keeps LTM and KCS
traffic separate (an LTM upload is filtered out before reaching KCS)
and also allows multiple GCE projects using gce-xfstests to share a
single GCS bucket without colliding on `xfstests-ltm` or `xfstests-kcs`
(sharing the single bucket notification and topic in the bucket's
owning project).  Internal RPCs between LTM and KCS
(`/internal` and `/internal-status`) do not go through the relay; the
Go servers talk directly to each other over their VPC internal IPs.

Parallel client invocations against the same VM are also supported:

* Each request carries a unique `<id>` (`<UTC timestamp>-<pid>-<64-bit
  random hex>`), giving every in-flight call its own `req/<id>.json`
  and `rsp/<id>.json` objects.  Cookie jars written by `gce-gcs-curl`
  are replaced atomically via a temporary file.
* `gce-gcs-relay` keeps its Pub/Sub pull loop on the main thread and
  dispatches requests across a worker thread pool (`--max-workers`,
  default 8).  Notifications are acknowledged on receipt and treated as
  untrusted hints; each worker validates the object, enforces the size
  and timeout limits, tracks in-flight `(name, generation)` pairs, and
  claims the object with an `ifGenerationMatch` delete before
  forwarding it to `https://127.0.0.1:443`.  A slow endpoint therefore
  does not head-of-line block other concurrent requests, duplicate or
  late Pub/Sub deliveries are harmlessly ignored, and `SIGTERM` waits
  for already-claimed requests to finish while leaving any unclaimed
  ones in `req/` for the next daemon's startup scan.
* The daemon only lists the bucket on startup, after `--response-ttl`
  (default 300 s) to remove unclaimed responses, while saturated, after
  recovering from a Pub/Sub error, or when invoked with `--once`.

### Inspecting or removing the Pub/Sub resources

To inspect the notification and subscriptions configured by
`gce-xfstests setup`:

        gcloud storage buckets notifications list gs://$GS_BUCKET
        gcloud pubsub subscriptions list --project="$GCE_PROJECT"
        gcloud pubsub subscriptions describe xfstests-ltm-gcs-relay --project="$GCE_PROJECT"

To tear them down (for example when decommissioning a project):

        gcloud pubsub subscriptions delete xfstests-ltm-gcs-relay xfstests-kcs-gcs-relay --project="$GCE_PROJECT"
        gcloud storage buckets notifications delete projects/_/buckets/$GS_BUCKET/notificationConfigs/<ID>
        gcloud pubsub topics delete gce-xfstests-gcs-relay --project="<topic-project>"

## The SSH tunnel wrapper (`gce-ssh-curl`)

As an alternative to the GCS relay when SSH access to the server VMs is
available, configuring `GCE_CURL=gce-ssh-curl` (see [Using an SSH tunnel
to reach the LTM and KCS
servers](gce-xfstests.md#using-an-ssh-tunnel-to-reach-the-ltm-and-kcs-servers))
causes `run_gce_curl` to invoke
[gce-ssh-curl](../run-fstests/util/gce-ssh-curl).  Unlike `gce-gcs-curl`,
nothing special runs on the appliance VM beyond `sshd`:

        curl  <-->  gce-ssh-curl --tunnel-daemon  <-->  ssh -L ...  <==>  sshd on VM
              unix                                unix                    --> 127.0.0.1:443
              socket                              socket

From the LTM or KCS server's point of view, connections arrive at
`127.0.0.1:443` just as with the GCS relay, while `curl` still speaks
HTTPS end-to-end over `--unix-socket` and validates the server's
certificate against `a.$GCE_PROJECT.gce-xfstests`.

### Tunnel daemon lifecycle, leases, and concurrency

* **Per-VM isolation and socket security**: Each `(VERSION, ssh_command,
  remote)` tuple hashes to its own `<instance>.<hash>.{sock,lock,log}`
  prefix inside a mode-`0700` directory (`$SSH_CURL_DIR`,
  `$XDG_RUNTIME_DIR/ssh-curl`, or `/tmp/ssh-curl-<uid>`).  Sockets are
  created with mode `0700` and the daemon checks `SO_PEERCRED` on every
  accepted connection so only the owning UID can talk to it.
* **Multiplexing control and TLS traffic on one socket**: Connections
  from the `gce-ssh-curl` wrapper begin with `CONTROL_MAGIC`
  (`\0ssh_curl `), whereas connections from `curl --unix-socket` begin
  with a TLS `ClientHello` record (whose first byte is never `0x00`),
  allowing a single Unix-domain socket to serve both control commands
  (`lease`, `status`, `stop`) and raw TLS stream forwarding to `ssh`'s
  `<base>.<pid>.fwd` local-forwarding socket.
* **Cold-start serialization and active leases**: Concurrent
  invocations when no daemon is running synchronize via `flock` on
  `<base>.lock` so only one background `--tunnel-daemon` and `ssh`
  process is started.  Before `exec`ing `curl`, the wrapper acquires a
  `lease` control connection whose file descriptor is marked inheritable;
  the lease stays open until `curl` exits.  The daemon considers itself
  busy whenever any control or forwarded connection is open (so
  `SSH_CURL_IDLE` never cuts off an in-flight request) and re-acquires
  `<base>.lock` non-blockingly before exiting on idle to avoid racing
  with a new caller.
* **Process-group and file-descriptor cleanup**: `ssh` (by default
  `gcloud compute ssh`, which spawns `ssh` as a child process rather than
  `exec`ing it) is started with `start_new_session=True` and
  `PR_SET_PDEATHSIG(SIGTERM)`.  Killing the daemon—even with
  `SIGKILL`—tears down `ssh`, and normal shutdown (`SIGTERM`, idle
  timeout, socket removal, or `--tunnel-stop`) signals the entire
  process group via `os.killpg`.  Once the forwarding socket is ready,
  the daemon redirects stdout and stderr to `<base>.log` so caller
  pipelines (`ltm_post_json` / `kcs_post_json`) never block on held pipes.

### Inspecting and debugging SSH tunnels

To list active tunnel daemons (including their PIDs, `ssh` PIDs, idle
timers, socket paths, and log file paths) or stop them cleanly:

        ./run-fstests/util/gce-ssh-curl --tunnel-status
        ./run-fstests/util/gce-ssh-curl --tunnel-stop

Each daemon's `<base>.log` records the exact `ssh`/`gcloud` command line
and any `ssh` diagnostic output (never request payloads or session
cookies).  Setting `SSH_CURL_VERBOSE=1` also prints client-side tunnel
status messages on stderr.

## Shared GCS bucket layout across multiple GCE projects

Multiple GCE projects belonging to the same trust boundary can share a
single `GS_BUCKET`:

* **Authoritative project discovery on VMs**: `gce-funcs` queries
  `project/project-id` from the GCE metadata server and caches it in
  `/var/lib/gce-xfstests/gce_project` so appliance scripts always know
  their own project ID regardless of what is in the bucket's root
  `gce_xfstests.config`.
* **Batch and startup directories**: `gce-ltm.boot` runs persistent boot
  scripts from `gs://$GS_BUCKET/ltm-rc/$GCE_PROJECT/`, and
  `gce-ltm-batch-watcher` polls one-shot scripts from
  `gs://$GS_BUCKET/ltm-batch/$GCE_PROJECT/` (logging output to
  `gs://$GS_BUCKET/ltm-batch-$GCE_PROJECT.log`).
* **Config and TLS certificates (legacy-slot ownership)**: To keep
  single-project upgrades silent across client/appliance skew, the
  project named in the bucket-root `gs://$GS_BUCKET/gce_xfstests.config`
  (or the first project to run `setup` on an empty bucket) owns the root
  `gce_xfstests.config` and `gce-xfstests-{cert,key,server}.pem` files
  as its primary location and mirrors them under
  `gs://$GS_BUCKET/project/$GCE_PROJECT/`.  Every additional project
  sharing the bucket uses `gs://$GS_BUCKET/project/$GCE_PROJECT/`
  exclusively and never writes to the root slot.  On boot,
  `gce-fetch-gs-files` (and `gce-setup`) uses the root slot when its
  `GCE_PROJECT` matches the metadata server, else
  `project/$GCE_PROJECT/` if present, else falls back to the root config
  with `GCE_PROJECT` forced to the metadata-server value.

## Testing the curl relays locally

Offline loopback self-test suites are located in
[selftests/curl-relay/](../selftests/curl-relay/README.md) and exercise
the `run_gce_curl` shell hooks, `gce-gcs-curl` + `gce-gcs-relay`, and
`gce-ssh-curl` (including against a local unprivileged `sshd` when
OpenSSH is installed) without touching real GCP or GCS resources:

        ./selftests/curl-relay/test_gcs_relay.py
        ./selftests/curl-relay/test_ssh_curl.py
