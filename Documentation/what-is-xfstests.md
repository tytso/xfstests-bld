# What is xfstests?

As the name might imply, xfstests is a file system regression test
suite which was originally developed by Silicon Graphics (SGI) for the
XFS file system.  Originally, xfstests, like XFS was only supported on
the SGI's Irix operating system.  When XFS was ported to Linux, so was
xfstests, and now xfstests is only supported on Linux.

Today, xfstests is used as a file system regression test suite for all
of Linux's major file systems: xfs, ext2, ext4, cifs, btrfs, f2fs,
reiserfs, gfs, jfs, udf, nfs, and tmpfs.  Many file system maintainers
will run a full set of xfstests before sending patches to Linus, and
will require that any major changes be tested using xfstests before
they are submitted for integration.

## Individal tests in xfstests

Tests in xfstests were originally named using a three digit number.
In 2013 the tests were moved into different classes, depending on
whether the test was file system specific, "generic" (meaning it was
was file system indepedent), or "shared" (meaning that test was not
truly generic, but which was useful on a handful of file systems).  In
this scheme, tests would have names such as:

* btrfs/126
* generic/013
* ext4/271
* shared/051
* xfs/090

Tests can be belong to multiple groups, such as "auto", "quick",
"aio", "prealloc", "ioctl", and "dangerous".  Membership in a group
can indicate something about the nature of the test.  The "auto" group
indicates those tests best suited for automatic test spinners.  Tests
in the "quick" group should completely quickly, and running the quick
group is good for a smoke test for the file system.  The "dangerous"
group are tests that can crash the kernel, and so they should be run
with care.

Other times, the group membership can indicate file system
functionality which is exercised by the test.  Examples of this would
include groups such as "aio", "prealloc", and "ioctl".

## Test devices

The xfstests test suite uses one or two block devices; one is named
TEST and must be present, and the other is named SCRATCH, and is
optional.  Most tests use either the TEST or the SCRATCH device,
although there are a few tests that use both devices.

The SCRATCH device is reformatted by tests which need to use the
SCRATCH device.  Individual tests may not assume that there is a valid
file system on the SCRATCH device.  In contrast, the TEST device is
never formatted by xfstests, and is intended to be a long lived,
"aged" file system.

For most ext4 file systems configurations, the TEST and SCRATCH device
should be 5GB.  Smaller, and some tests may not run correctly.
Larger, and the tests will take a long time to run --- especially
those tests that need to fill the file system to test ENOSPC handling.
There are a few file system configurations for ext4 (most notable,
bigalloc) which require a 20GB test and scratch device.

For this reason, kvm-xfstests uses five file system devices, /dev/vdb,
/dev/vdc, /dev/vdd, /dev/vde, and /dev/vdf.  (/dev/vda is used for the
root file system, and /dev/vdg is used for the /results file system.)
The first two test devices, /dev/vdb and /dev/vdc, are used for TEST
and SCRATCH, respectively, for the standard, default 4k file system
configuration.  The /dev/vdd device is used for file system
configurations which are not compatible with the default 4k block ext4
file system --- for example, a 1k block file system.  Since we want to
keep the /dev/vdb as a long-term file system test file system aging,
we use /dev/vdd instead for the 1k block file system, and the test
runner will run mke2fs to format /dev/vdd before starting the xfstests
run for that file system configuration.  For this reason, /dev/vdd is
sometimes called the TEST-1K device -- although there are many other
file system configurations which will use /dev/vdd.

The /dev/vde and /dev/vdf file systems are the BIGTEST and BIGSCRATCH
disks, and are used for those file system configrations which require
a 20GB TEST and SCRATCH device.  Like the TEST-1K device, the BIGTEST
device will be reformated using mke2fs at the beginning of an xfstests
run for that file system configuration.

The gce-xfstests uses the same set of block devices, although instead
of individual virtio devices, gce-xfstests uses Logical Volumes using
LVM stored on a GCE Local SSD.  Unfortunately, this means that the
TEST-4K device is reformatted each time the gce-xfstests VM is
started; and so we don't get the benefits of testing the file system
against a device suffering from long-term aging.
