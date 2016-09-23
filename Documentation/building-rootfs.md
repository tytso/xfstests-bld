# Building the root_fs.img file

The root_fs.img file is used as the test appliance VM for
kvm-xfstests.  It consists of a Debian Jessie root file system, some
configuration files and test runner scripts, and the xfstests.tar.gz
unpacked in the /root directory.  It is built using the gen-image
script found in kvm-xfstests/test-appliance.

The gen-image script must be run as root, as it creates a file system
and mounts it as part of the root_fs construction process.

## Adding additional packages

There are two ways to add additional packages to the root_fs image.
The first is to supply the package name(s) on the command line, using
the -a option.

The second is to copy the debian packages into the directory
kvm-xfstests/test-appliance/debs.  This is how the official packages
on kernel.org have an updated version of e2fsprogs and its support
packages (e2fslibs, libcomerr2, and libss2).  The latest versions get
compiled for Debian Jessie, in a hermetic build environment, and
placed in the debs directory.  Optionally, the output of the script
[get-ver](https://git.kernel.org/cgit/fs/ext2/e2fsprogs.git/tree/util/get-ver)
is placed in the e2fsprogs.ver in the top-level directory of
xfstests-bld.  This gets incorporated into the git-versions file found
in the xfstests.tar.gz file, so that there will be a line like this in
the file:

        e2fsprogs       v1.43.1-22-g25c4a20 (Wed, 8 Jun 2016 18:11:27 -0400)

If you don't want to compile your own, very latest version of
e2fsprogs, there is a newer version of e2fsprogs, compiled for the
Debian Jessie distribution,
[available](https://packages.debian.org/jessie-backports/admin/e2fsprogs)
in the debian backports of the archive.
