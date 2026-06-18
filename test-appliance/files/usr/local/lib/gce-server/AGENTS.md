# gce-server

This directory contains the backend services that provide support for
distributed testing and automated kernel builds for `gce-xfstests`.
Those two services are the Lightweight Test Manager (ltm), and Kernel
Compilation Service (kcs).

## Directory Structure

- `ltm/`: Lightweight Test Manager. Orchestrates test execution and VM
  sharding.

- `kcs/`: Kernel Compilation Service. Handles kernel building and git
  bisection.

- `util/`: Shared Go packages used by both LTM and KCS.

---

## Services

### 1. Lightweight Test Manager (ltm)

The ltm runs on a persistent management VM in GCE. It is the central
coordinator for test requests.

- **Request Handling**: Exposes `/gce-xfstests` to receive test
    requests from the client-side `gce-xfstests` script.

- **Distributed Testing (Sharding)**: Uses `ShardScheduler` to parse
    test requests, query GCE quotas, and automatically provision
    multiple worker VMs (`ShardWorker`). Tests are distributed across
    these shards to run in parallel.

- **Result Aggregation**: Monitors running shards, retrieves test logs
    and results, aggregates them into a consolidated summary, and
    emails the report to the user.

- **Git Repo Monitoring**: Supports a "watch" mode (`GitWatcher`) that
    monitors specific Git branches and automatically triggers new test
    runs when changes are detected.

- **Inter-service Communication**: Forwards kernel build and git
    bisection requests to kcs.

### 2. Kernel Compilation Service (kcs)

The kcs runs on a dedicated utility VM, acting as a compilation and
bisection engine.

- **Kernel Building**: Receives requests via `/gce-xfstests` or
    `/internal` to build a kernel. It clones the target repository,
    applies configuration options, compiles the kernel, packages it,
    and uploads the resulting image to Google Cloud Storage (GCS).

- **Automated Git Bisection**: Supports automated git bisection
    (`RunBisect`) to locate commits that introduced test
    regressions. KCS compiles the kernel for each bisection step and
    coordinates with LTM to run tests on that kernel.

- **Lifecycle Management (Auto-shutdown)**: To minimize costs, KCS
    monitors its own activity. If it remains idle for more than 1 hour
    (tracked by `StartTracker`) and has no active bisection tasks, it
    automatically shuts down and deletes its own VM.

---

## Shared Utilities (`util/`)

The `util` directory contains modular Go packages that provide the foundation for both services:

- **`gcp`**: Wrapper around Google Cloud APIs for managing GCE
    instances (creation, deletion, metadata modification) and GCS
    buckets.

- **`git`**: Handles local Git operations including cloning, checkout,
    and orchestrating the kernel build and upload process.

- **`server`**: Framework for HTTP servers, routing, error handling,
    and session authentication.

- **`email`**: Handles sending test and build reports to users.

- **`parser`**: Parses `xfstests` command-line arguments and
    configuration options into structured formats.

- **`check` / `logging` / `mymath`**: Common helpers for error
    handling, logging (via `logrus`), and timestamp generation.

---

## Key Workflows

### 1. Running a Test Suite
```
[Client (gce-xfstests)] -> (HTTP POST /gce-xfstests) -> [LTM Server]
                                                            |
                                                 (Provisions & coordinates)
                                                            v
                                                    [Shard Worker VMs]
```
1. User invokes `gce-xfstests` locally.
2. The client script sends a JSON request to LTM.
3. LTM's `ShardScheduler` splits the tests into parallel shards and
   spins up worker VMs using a copy of the `gce-xfstests` script running
   on the ltm VM.
4. Worker VMs run the tests and upload results.
5. LTM aggregates results and emails the user.

### 2. Building a Kernel / Git Bisect
```
[Client / LTM] -> (HTTP POST /internal) -> [KCS Server]
                                               |
                                     (Builds & uploads to GCS)
                                               v
                                         [GCS Bucket]
```
1. If a test run requires a custom kernel build or bisect, ltm
   forwards the request to kcs.
2. kcs builds the kernel and uploads it to GCS.
3. Once the build completes, ltm provisions workers using the new
   kernel from GCS to run the tests.
