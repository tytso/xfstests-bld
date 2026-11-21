# selftests

This directory contains self-tests for the xfstests build system, test
appliance generation, and GCE orchestration tools (LTM/KCS).

## Configuration

Configuration is defined in the `config` file and can be overridden in
the `config.custom` file in the `selftests` directory.

Key variables:
- `KSRC`: Path to the kernel source tree used for test builds (defaults to `/usr/src/linux`). **You should override this in `config.custom` to point to your kernel sources.**
- `DISTRO`: Debian distribution used for building test appliances (defaults to `trixie`).
- `PRIMARY_FSTYPE`: Primary filesystem type to test (defaults to `ext4`).

## Test Scripts

The self-tests consist of three main scripts that should generally be run in the following order:

### 1. build-kernel
Tests the kernel build infrastructure (`install-kconfig` and `kbuild`).
- **Behavior**: Builds kernels for specified architectures (default: `arm64`, `i386`, `amd64`).
- **Outputs**: Copies built kernel packages and images to **kernels** directory:
  - `kernels/kernel-$ARCH.deb`
  - `kernels/Image-$ARCH`

### 2. appliance
Tests building and running the test appliance images.
- **Prerequisites**: Requires kernels built by `build-kernel` to be present in `selftests/kernels/`. Requires build chroots (e.g., `trixie-$ARCH`) to exist on the host.
- **Behavior**:
  - Builds test appliance rootfs images using `build-appliance` script.
  - Tests KVM appliance by running a quick test (`generic/001`) via `kvm-xfstests` and verifying pass.
  - (Optional) If `~/.config/gce-xfstests` exists, builds GCE images, launches test VMs, runs tests, and verifies results.

### 3. ltm-kcs
Tests the Lightweight Test Manager (LTM) and Kernel Compilation Service (KCS) in GCE.
- **Prerequisites**: Requires GCE setup and kernels in `selftests/kernels/`.
- **Behavior**:
  - Shuts down existing LTM.
  - Uploads prebuilt kernel deb (tests LTM path).
  - Triggers build from git repo/commit (tests KCS path).
  - Launches LTM with a batch file and waits for results in GCS.
  - Verifies test results.

### 4. gcs-relay/test_gcs_relay.py
Offline loopback self-test suite for `GCE_CURL`, `run-fstests/util/gce-gcs-curl`,
and `test-appliance/files/usr/local/lib/gce-gcs-relay`.
- **Prerequisites**: Python 3 (stdlib only), `bash`, `curl`, `jq`, and `openssl`. No GCP credentials, bucket, VMs, or network access required.
- **Usage**: Can be invoked from any working directory (auto-detects the repo root relative to itself, or accepts `--repo PATH`). Supports `--list`, `-k SUBSTRING`, `-v`, and `--keep`.
- **Behavior**: Starts local fakes (`fake_gcp.py`, `fake_gcloud.py`, `fake_ltm.py`) with an ephemeral self-signed CN-only cert and tests default `curl`, `GCE_CURL` hooking, LTM/KCS routing isolation, multi-project bucket isolation, concurrent requests, HTTP 412 metadata fingerprint conflicts, cookie-jar interop, error/timeout handling, and `SIGTERM` draining.

