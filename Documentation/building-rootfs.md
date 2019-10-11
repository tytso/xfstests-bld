# Building the test appliance

## Overview

For kvm-xfstests, a `root_fs.img` disk image file is used as the test
VM's root filesystem, while for android-xfstests the tests are run in
a chroot directory created by unpacking a file `root_fs.tar.gz`.  Both
types of `root_fs` contain a Debian root filesystem, some
configuration files and test runner scripts, and the `xfstests.tar.gz`
unpacked in the `/root` directory.

Briefly, building either type of `root_fs` requires setting up a
Debian build chroot and building the xfstests tarball as described in
[building-xfstests](building-xfstests.md), then running the
`gen-image` script.  The `do-all` script can automate this process
slightly, as described below.

## Using a proxy

In order to properly utilize a proxy you need to make sure to add the
following line (replacing server:port with your actual settings)

    export http_proxy='http://server:port'

to config.custom in both the root directory of your xfstest-bld checkout
and to kvm-xfstests/test-appliance.

## Using gen-image

After building the xfstests tarball as described in
[building-xfstests](building-xfstests.md), a `root_fs` may be built
using the `gen-image` script found in `kvm-xfstests/test-appliance/`.
By default `gen-image` builds a `root_fs.img`; in this case,
`gen-image` must be run as root, since it creates a filesystem and
mounts it as part of the `root_fs` construction process.  To build a
`root_fs.tar.gz` instead, pass the `--out-tar` option.

Example:

    cd kvm-xfstests/test-appliance
    sudo ./gen-image

## Using the do-all convenience script

To more easily build a test appliance, you can use the `do-all`
convenience script.  `do-all` will build the xfstests tarball, then
invoke `gen-image` to build a `root_fs`.  It allows specifying the
build chroot to use as well as whether a `root_fs.img` or
`root_fs.tar.gz` should be created.

For kvm-xfstests, use one of the following commands to create an i386
or amd64 test appliance, respectively:

    ./do-all --chroot=buster-i386  --no-out-tar
    ./do-all --chroot=buster-amd64 --no-out-tar

For android-xfstests, use one of the following commands to create an
armhf or arm64 test appliance, respectively:

    ./do-all --chroot=buster-armhf --out-tar
    ./do-all --chroot=buster-arm64 --out-tar

The build chroot(s) can be created using the `setup-buildchroot`
script as described in [building-xfstests](building-xfstests.md).
Note that you do not need to be running an ARM system to create the
ARM test appliances, since the `setup-buildchroot` script supports
foreign chroots using QEMU user-mode emulation.

You may also set the shell variables `BUILD_ENV`, `SUDO_ENV`, and/or
`OUT_TAR` in your `config.custom` file to set defaults for `do-all`.
For example, if you'd like to default to building an amd64
kvm-xfstests appliance, use:

    BUILD_ENV="schroot -c buster-amd64 --"
    SUDO_ENV="schroot -c buster-amd64 -u root --"
    OUT_TAR=

## Adding additional packages

There are two ways to add additional packages to the root_fs image.
The first is to supply the package name(s) on the command line, using
the -a option.

The second is to copy the debian packages into the directory
kvm-xfstests/test-appliance/debs.  This is how the official packages
on kernel.org have an updated version of e2fsprogs and its support
packages (e2fslibs, libcomerr2, and libss2).  The latest versions get
compiled for Debian Stretch, in a hermetic build environment, and
placed in the debs directory.  Optionally, the output of the script
[get-ver](https://git.kernel.org/cgit/fs/ext2/e2fsprogs.git/tree/util/get-ver)
is placed in the e2fsprogs.ver in the top-level directory of
xfstests-bld.  This gets incorporated into the git-versions file found
in the xfstests.tar.gz file, so that there will be a line like this in
the file:

        e2fsprogs       v1.43.1-22-g25c4a20 (Wed, 8 Jun 2016 18:11:27 -0400)

If you don't want to compile your own, very latest version of
e2fsprogs, there is a newer version of e2fsprogs, compiled for the
Debian Stretch distribution,
[available](https://packages.debian.org/stretch-backports/admin/e2fsprogs)
in the debian backports of the archive.
