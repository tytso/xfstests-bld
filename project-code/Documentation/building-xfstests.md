# Building xfstests

The xfstests-bld package makes it easy to build xfstests in a hermetic
build environment (so it is not dependent on possibly out-of-date
distro versions of libaio, xfsprogs, etc.).  This was the original
raison d'etre for xfstests-bld, which explains why it was so named.

## Fetching the external git trees

The xfstests-bld package depends on a number of external git trees:

* xfstests-dev
* xfsprogs-dev
* fio
* quota
* fsverity
* ima-evm-utils

The first time you run "make", the build system will clone these
repositories by running ./get-all.  Their remote URLs are set in the
top-level "config" file.  If you wish to make changes, copy "config"
to "config.custom" and make changes there.

The config file can also specify the commit to use for each
repository.  If a commit is specified, the build system will check it
out after cloning the repository.  The commit will also be checked out
each time a new build is done, in case the config file was changed to
specify a different commit.  Note that this will override any local
changes.  If, on the other hand, no commit is specified, then the
repository will simply start out at the latest "master", and you will
be free to make local changes or update it with "git pull" as desired.

(Please take care before updating the fio repository; some updates to
the fio tree have caused test regressions in the past, so it may be
preferable to let things be as far as the fio repo is concerned.)

The build also supports some optional repositories which are only
included when their URLs are uncommented in the config file; see the
config file for a full list.

## Choosing a build type

xfstests-bld can be used to build xfstests and its dependencies for
one of the official "test appliances"
([kvm-xfstests](kvm-xfstests.md), [gce-xfstests](gce-xfstests.md),
[android-xfstests](android-xfstests.md), etc.), then optionally build
the test appliance itself.  This normally requires using a Debian
build chroot.  Alternatively, xfstests-bld can be used to build
xfstests and its dependencies for a different environment, such as for
the native system, without using a build chroot.

### Preparing a build chroot

To build xfstests and its dependencies for one of the official test
appliances, it is strongly recommended to use a Debian build chroot.
Using a build chroot ensures that an appropriate compiler toolchain is
used and that the binaries are linked to the appropriate shared
libraries.  In addition, using QEMU user-mode emulation it is possible
to create a chroot for a foreign architecture, making it easy to do
cross-architecture builds.

To set up a Debian build chroot, run the `setup-buildchroot` script.
`setup-buildchroot` will invoke `debootstrap` to bootstrap a minimal
Debian system into a directory (by default a subdirectory of
`/chroots/`), then set it up for use with `schroot` and install into
it all the Debian packages needed for the build.  `setup-buildchroot`
must be run as root, since it needs root permission to run
`debootstrap` and update `/etc/schroot/schroot.conf`.

`setup-buildchroot` supports setting up a chroot using any
architecture supported by Debian.  For kvm-xfstests appliances, you'll
need either an i386 or amd64 chroot:

    $ sudo ./setup-buildchroot --arch=i386
    $ sudo ./setup-buildchroot --arch=amd64

For gce-xfstests test appliances, you'll need an amd64 chroot:

    $ sudo ./setup-buildchroot --arch=amd64

For android-xfstests test appliances, you'll need an armhf or arm64
chroot:

    $ sudo ./setup-buildchroot --arch=armhf
    $ sudo ./setup-buildchroot --arch=arm64

Normally ARM will be a foreign architecture, so `setup-buildchroot`
will walk you through installing the needed QEMU binary and
binfmt_misc support to get QEMU user-mode emulation working.
Afterwards, it will behave just like a native chroot.

Once you're created a chroot, you should be able to use the `schroot`
program to enter it, e.g.:

    $ schroot -c buster-amd64         # enter chroot as regular user
    $ schroot -c buster-amd64 -u root # enter chroot as root

The `-c` option must specify the name of the chroot as listed in
`/etc/schroot/schroot.conf`.  By default `setup-buildchroot` names the
chroots after the Debian release and architecture.

### Without a build chroot

If you want to use xfstests-bld without a dedicated build chroot, a
number of prerequisite packages are needed.  They can be installed
using the following command:

    $ sudo apt-get install autoconf autoconf2.64 \
		automake autopoint bison build-essential ca-certificates \
		debootstrap e2fslibs-dev ed fakechroot gettext git \
		libdbus-1-3 libgdbm-dev libkeyutils-dev libssl-dev \
		libblkid-dev libtool-bin pkg-config qemu-utils uuid-dev \
		rsync symlinks lsb-release golang-1.8-go

It is also possible to use a cross compiler rather than the native
compiler.  To do this, set the shell variables `CROSS_COMPILE` and
optionally `TOOLCHAIN_DIR` in your `config.custom` file as follows:

* `CROSS_COMPILE` should be set to the target triplet.  For example,
  on a Debian system, you can install the `gcc-arm-linux-gnueabihf`
  package and then set `CROSS_COMPILE=arm-linux-gnueabihf` to cross
  compile for the Debian armhf platform.

* `TOOLCHAIN_DIR` can be set to the directory containing the
  cross-compiler toolchain, if it is not already on your `$PATH`.  It
  should specify the directory one level above the `bin` directory
  containing the compiler executable file.

## Building the xfstests tarball

You may skip explicitly building the xfstests tarball if you are using
the `do-all` convenience script to build a test appliance, as
described in [building-rootfs](building-rootfs.md).  Otherwise, you
can build the tarball as follows:

    $BUILD_ENV make clean
    $BUILD_ENV make
    $BUILD_ENV make tarball

... where `BUILD_ENV` should be set to `"schroot -c $CHROOT_NAME --"`
for a chroot build environment (where `$CHROOT_NAME` should be
replaced with the name of the chroot as listed in
`/etc/schroot/schroot.conf`) or an empty string otherwise.

Briefly, these `make` targets do the following tasks:

* `make clean` cleans the various components.
* `make` (or equivalently `make all`) builds the various subcomponents
  and installs them into the `bld` directory.
* `make tarball` copies the files actually needed to run xfstests into
  the `xfstests` scratch directory, then creates `xfstests.tar.gz` by
  running the `gen-tarball` script.
