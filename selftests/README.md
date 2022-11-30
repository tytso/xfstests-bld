# Selftests for xfstests-bld

This directory contains self-tests for xfstests-bld.  There are
currently three self-test scripts, which generally should be run in
this order:

* build-kernel
* appliance
* ltm-kcs

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
