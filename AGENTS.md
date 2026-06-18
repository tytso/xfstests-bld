# xfstests-bld — Agent Guide

## What this is

1.  Build system for `xfstests` (the primary filesystem test suite for
Linux) and its dependencies, resulting in a xfstests.tar.gz tarball
file.  Located in the fstests-bld directory.

2.  Creation of a test-appliance which can be using qemu/kvm, in a
Google Compute Engine (GCE) VM, and in Android.  The test appliance
and the tools to build it are found in the test-appliance directory.
The client front-end programs (kvm-xfstests, gce-xfstests, and
android-xfstests) are found in the run-fstests directory.

3.  A lightweight test manager (ltm) management VM and a kernel
compilation service (kcs) utility VM which is found in the
`test-appliance/files/usr/local/lib/gce-server` directory.

4.  Infrastructure for building Linux kernels and creating a kernel
configuration file which is suitable for use by the test appliance is
found in the kernel-build directory.
