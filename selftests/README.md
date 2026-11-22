# Selftests for xfstests-bld

This directory contains self-tests for xfstests-bld.  There are
currently three main build/runtime self-test scripts (which generally
should be run in order), plus two standalone offline curl-relay suites:

* build-kernel
* appliance
* ltm-kcs
* curl-relay/test_gcs_relay.py
* curl-relay/test_ssh_curl.py

These self-test scripts should be run out of the top-level directory
of xfstests-bld or the alternatively, the self-tests directory.

## build-kernel

This test script tests the install-kconfig and kbuild scripts.  It
also copies the built kernels into the selftests/kernels directory,
which are needed for the subsequent scripts.

The selftests/config script sets the default location of the kernel
sources to /usr/src/linux, but in general, the developer should set
kernel sources that should be used in the selftests/config.custom
file.  For example:

    KSRC=/usr/projects/linux/ext4-5.15

## appliance

This test script will build and run a quick self-test for the test
appliances for kvm-xfstests and gce-xfstests.  If the file
~/.config/gce-xfstests exist, then it will also build and test
gce-xfstets appliance images.

By default, the selftests/appliance script will build test appliances
for arm64, amd64, and i386 for kvm-xfstests, and amd64 and arm64 for
gce-xfstests.

## ltm-kcs

This test script will run self-tests for the LTM and KCS server.

## curl-relay

`curl-relay/test_gcs_relay.py` runs local, offline end-to-end self-tests
for `GCE_CURL`, `run-fstests/util/gce-gcs-curl`, and
`test-appliance/files/usr/local/lib/gce-gcs-relay` against loopback
fakes of GCS, the GCE metadata server, `gcloud`, and LTM/KCS.

`curl-relay/test_ssh_curl.py` runs local, offline end-to-end self-tests
for `GCE_CURL` and `run-fstests/util/gce-ssh-curl` against loopback
fakes of `ssh` and LTM/KCS (plus a local OpenSSH `sshd` when installed).

Neither suite requires GCP access or built kernels.  See
[curl-relay/README.md](curl-relay/README.md) for options and details.
