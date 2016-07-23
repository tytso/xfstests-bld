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

The location of these files are specified in the top-level config
file, but you can copy the config file to config.custom and then make
changes if desired.

The first time you run "make", the scripts will automatically fetch
these git trees from the locations specified in the top-level config
file.  You can also manually run the "get-all" script which will
actually do the dirty deed.

There may be updates in some or any of these git trees for these
subcomponents.  You can use "git pull" or "git fetch" as necessary to
update them.  (Please take care before updating the fio repository;
some updates to the fio tree have caused test regressions in the past,
so it may be preferable to let things be as far as the fio repo is
concerned.)

## Building the xfstests tarball

1.  Run "make clean"

2.  Run "make".  This will run autoconf (if necessary) in the various
subcompoents, run "make" or the equivalent to build all of the
subcomponents, and then finally run "make install" to install the
build artifacts into the bld directory.  The actual work is done via
the "build-all" script.

3.  Run "make tarball".  This will copy the files actually needed to
run xfstests into the xfstests scratch directory, and then create the
xfstests.tar.gz file.  The actual work is done by the "gen-tarball"
script.

## Build environments for xfstests

There are three important aspects of the environment in which the
xfstests binaries are built.

* The build utilities: autoconf, automake, libtool, etc.
* The compiler toolchain: gcc, binutils, ranlib, strip, etc.
* The (shared) libraries used for linking the executables

In practice, the largest impact will be the compiler toolchain; and on
the x86 platform, whether 32-bit or 64-bit binaries are generated.

The subsections listed below are optional, in that if you are only
interested in building 64-bit x86 binaries to be run on a 64-bit x86
kernel, you can probably use your desktop Linux environment to build
the xfstests.tar.gz file.  However, there reasons why you may want to
use a more sophisticated way of building xfstests.

For example, by default xfstests-bld will link the binaries statically
to avoid problems between the build environment and the runtime
environment.  However, statically linked binaries are significantly
larger.  Using a chroot environment to guarantee that the runtime and
build environments are in sync results in substantial space savings
(almost an order of magnitude) since it becomes safe to use
dynamically linked executables.


### Building in a chroot environment

These instructions assumes you are using Debian; they should probably
work for Ubuntu as well.

If you want to build a 64-bit test image, just remove the --arch=i386
in step #3, and use a schroot name of "jessie-64" instead of
"jessie-32".

1. Install the necessary packages to build host OS

        % sudo apt-get install schroot debootstrap

2.  Add the following to /etc/schroot/schroot.conf, replacing "tytso"
with your username, and /u1/jessie-32 with path where you plan to
put your build chroot

        [jessie-32]
        description=Debian Jessie 32-bit
        type=directory
        directory=/u1/jessie-32
        users=tytso,root
        root-users=tytso

3. Create the build chroot (again, replace /u1/jessie-root with the
pathname to your build chroot directory):

        % cd /u1
        % sudo debootstrap --arch=i386 jessie /u1/jessie-32
        % schroot -c jessie-32 -u root
        (jessie-32)root@closure:/u1# apt-get install build-essential autoconf autoconf2.64 automake libgdbm-dev libtool-bin qemu-utils gettext e2fslibs-dev git debootstrap fakechroot libdbus-1-3 autopoint pkg-config symlinks
        (jessie-32)root@closure:/u1# exit

4. Copy config to config.custom, and then change the lines which
define SUDO_ENV and BUILD_ENV to:

        SUDO_ENV="schroot -c jessie-32 -u root --"
        BUILD_ENV="schroot -c jessie-32 --"

5. Kick off the build!

        ./do-all


### Using an alternate compiler toolchain

A common reason for using an alternate compiler toolchain is to allow
you to cross-compile xfstests for a different target architecture.
This is done by setting the CROSS_COMPILE and TOOLCHAIN_DIR shell
variables in the top-level config file, or (this is preferable) in the
config.custom file.

To use an alternate toolchain, the shell variable CROSS_COMPILE
should be set to the target architecture.  For example, on a Debian
stretch system, you can install the gcc-arm-linux-gnueabihf to build package
and then set CROSS_COMPILE to "arm-linux-gnueabihf" to cross compile
for the Debian armhf platform.

The TOOLCHAIN_DIR shell variable can be used to specify the location
for the alternate compiler toolchain if it is not your path.  For
example, let's assume you've install the Android Native Delevelopment
Toolkit (NDK) and used the make-standalone-toolchain.sh to install a
toolchain in /u1/arm64-toolchain.  (See the [Android NDK
documentation](https://developer.android.com/ndk/guides/standalone_toolchain.html)
for more information.)  To use the standalone toolchain designed for
Android, configure TOOLCHAIN_DIR to /u1/arm64-toolchain and
CROSS_COMPILE to aarch64-linux-android.

### Instructions for building an armhf root_fs.tar.gz file

The armhf_root_fs.tar.gz file is used for testing file systems on
Android devices, and was generated as follows:

1.  Copy the xfstests-bld git tree to a debian build host running the
armhf platform.

2.  Set up a Debian Stable (Jessie) build environment and enter it.  For
example, if you are doing this on a Debian build server, assuming you
are a Debian developer with access to the Debian build architecture (I
was using harris.debian.org)

        schroot -b -c jessie -n tytso-jessie
        dd-schroot-cmd -c tytso-jessie apt-get install build-essential \
                autoconf autoconf2.64 automake libgdbm-dev libtool-bin \
                qemu-utils gettext e2fslibs-dev git debootstrap \
                fakechroot libdbus-1-3 autopoint pkg-config symlinks
        schroot -r -c tytso-jessie
Alternatively, make sure the build system is installed with Debian
Stable (e.g., Jessie), and install the following packages:

        % apt-get install build-essential build-essential \
                autoconf autoconf2.64 automake libgdbm-dev libtool-bin \
                qemu-utils gettext e2fslibs-dev git debootstrap \
                fakechroot libdbus-1-3 autopoint pkg-config symlinks

3.  Build the xfstests.tar.gz file (which contains the actual xfstests binaries built for armhf)

        cd xfstests-bld
        make
        make tarball

4.   Create the root_fs.tar.gz chroot environment

        cd kvm-xfstests/test-appliance
        ./gen-image --out-tar

5.  If you are on a Debian build server, clean up after yourself.

        schroot -e -c tytso-jessie
        rm -rf /home/tytso/xfstests-bld
