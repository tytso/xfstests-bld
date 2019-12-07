# Running xfstests on Android

## Introduction

android-xfstests runs xfstests on the internal storage of an Android
device, offering an interface similar to
[kvm-xfstests](kvm-xfstests.md) and [gce-xfstests](gce-xfstests.md).
It uses adb and fastboot to control the device, and it runs the tests
in a Debian chroot.  (The chroot is needed for compatibility with
xfstests and the various helper programs it invokes.)
android-xfstests should only be run on a device on which you don't
mind all user data being deleted.

Currently, android-xfstests has only been tested on a small number of
devices.  If you encounter a problem, please submit a fix!

## Requirements

- The android-xfstests script installed:
  run `make android-xfstests.sh` in the top-level directory of
  xfstests-bld, then move `android-xfstests.sh` to
  `~/bin/android-xfstests` or another location on your `$PATH`.

- A rooted Android device with sufficient internal storage.  For most
  test configurations, about 24 GiB of internal storage should be
  sufficient.  This is the sum of three 5 GiB partitions, a shrunken 4
  GiB userdata partition, and the various other partitions used by
  Android devices.  For test configurations requiring large
  partitions, like bigalloc, you'll need about 64 GiB instead.

- Ability to connect to the Android device with adb and fastboot.
  Usually this is done via a USB cable.

- An armhf or arm64 Debian root filesystem set up with xfstests and
  the xfstests-bld scripts.  Either fetch the prebuilt
  `root_fs.arm64.tar.gz` from
  [kernel.org](http://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests),
  or build one yourself as described in
  [building-rootfs](building-rootfs.md).  Then, either put the chroot
  tarball in the default location of
  `kvm-xfstests/test-appliance/root_fs.tar.gz`, or specify it with
  `ROOT_FS` in your `~/.config/android-xfstests` or the `-I` option to
  android-xfstests.

## Procedure

### (Optional) Build a custom kernel

You should be able to run xfstests with the kernel already installed
on your device, but you may wish to build your own kernel instead.
The exact procedure for building the kernel is device-dependent, but
here are example commands for building a kernel using the public
source code for the Google Pixel phone (2016 edition), code name
"marlin":

    git clone https://android.googlesource.com/kernel/msm msm-linux
    cd msm-linux
    git checkout android-msm-marlin-3.18-nougat-mr1
    export CROSS_COMPILE=aarch64-linux-android-
    export ARCH=arm64
    make marlin_defconfig
    make -j$(grep -c processor /proc/cpuinfo)

This will produce a kernel image `arch/arm64/boot/Image.gz-dtb`.

Also consider the following config options:

* `CONFIG_SYSV_IPC=y`:  Makes several tests stop failing / being
  skipped (see [Known issues](#known-issues))

* `CONFIG_CRYPTO_MANAGER_DISABLE_TESTS=n`: Include crypto self-tests.
  This may be useful if you are using xfstests to test file-based
  encryption.

### (Optional) Boot into your custom kernel

By default, android-xfstests uses the kernel running on the device.
However, it also supports booting a kernel automatically.  To do this,
specify `KERNEL` in your `~/.config/android-xfstests` or use the
`--kernel` command line option.  If a kernel is specified,
android-xfstests will boot it on the device using `fastboot boot`.  As
an optimization, this is skipped if the kernel is already running on
the device (as detected by checking `uname -r -v`; this usually
identifies the commit and build timestamp).

Note that `fastboot boot` just boots the kernel once; it doesn't
install it persistently.  Also, on some devices it doesn't bring up
the full functionality or even doesn't work at all, since it doesn't
install kernel modules or device-tree overlays.  If it doesn't work,
you'll need to install and boot the kernel yourself instead.  See your
device's documentation for details, but you'll at least need to build
the kernel into a boot.img and flash it to the "boot" partition.  On
some devices additional partitions must be flashed as well, e.g.
vendor and vbmeta.  As an example, for "marlin" devices running
Android N, one must copy `arch/arm64/boot/Image.gz-dtb` into
`device/google/marlin-kernel/` in the Android source tree, then run
the following commands from the root of the Android source tree:

    . build/envsetup.sh
    lunch marlin-eng
    make -j16 bootimage
    fastboot flash boot out/target/product/marlin/boot.img
    fastboot continue

### Running tests

To run tests, first ensure your device is connected with adb, then run
a command like the following:

    android-xfstests -c 4k -g auto

The options accepted by android-xfstests are generally the same as
those accepted by [kvm-xfstests](kvm-xfstests.md).  However, some
options do not apply or are not yet implemented.

If you have never before run android-xfstests on the device, then
android-xfstests will first need to resize the userdata filesystem to
make room for the xfstests partitions.  Currently, this is implemented
by rebooting into fastboot mode and reformatting the userdata
filesystem.  Since this causes all user data to be wiped,
android-xfstests will ask for confirmation before doing this.

Note that Android devices usually run an older version of the Linux
kernel.  At the same time, xfstests is constantly being updated to add
new tests.  Therefore, it's often the case that some of the more
recently added tests will fail.  Some tests may even cause a kernel
crash or deadlock and will need to be excluded with `-X` in order for
the test run to complete, as a temporary workaround until you can
backport the needed bug fixes.  It's recommended to keep your device's
kernel up-to-date with the corresponding LTS kernel to help minimize
the number of test failures that need to be triaged.  (Of course,
there are obviously many other benefits of doing that as well...)

## Monitoring and debugging

To get a shell in the chroot, use `android-xfstests shell`.  You can
do this at any time, regardless of whether tests are currently
running.  Note that this is a real shell on the device, and it doesn't
use a snapshot of the root filesystem like `kvm-xfstests shell` does.
Thus, any changes you make in the shell session are persistent.

## Known issues

If using the armhf (32-bit) tarball on an aarch64 kernel, the
encryption tests may fail due to a kernel bug that caused the keyctl
system call to be unavailable to 32-bit programs.  This can be fixed
by cherry-picking commit 5c2a625937ba ("arm64: support keyctl() system
call in 32-bit mode") into your kernel from Linux v4.11.

Tests that create device-mapper devices (e.g. generic/250,
generic/252, generic/338) fail because the Android equivalent of udev
does not create the dm device nodes in the location expected by
xfstests (`/dev/mapper/`).

Android kernels are sometimes configured without SysV IPC support ---
i.e., `CONFIG_SYSVIPC` isn't set.  This can cause several problems:

- Tests that use the `dbench` program (generic/241) fail.
- Tests that use the `dmsetup` program fail (if they didn't already
  fail because of the device node issue noted above)
- Tests that use the `fio` program (e.g. ext4/301, ext4/302, ext4/303,
  generic/095, generic/299, generic/300) are skipped.

generic/004 should work, but in fact it's skipped because of a [glibc
bug](https://sourceware.org/bugzilla/show_bug.cgi?id=17912).

## Resetting userdata

The partitions set up by android-xfstests are transient, so they will
not show up in the device's on-disk partition table or fstab, and they
will go away after reboot.  However, the reformatting of the userdata
filesystem with a smaller size to free up space is persistent.  If you
are done running xfstests and wish to expand userdata to take up its
full partition again, then reboot into fastboot mode and run `fastboot
-w` to wipe and reformat userdata again, this time with the full size.

## Other notes

Note that xfstests does pretty heavy I/O.  It is also possible to run
xfstests on external storage, e.g. on a USB-attached SSD.  However,
android-xfstests currently only supports internal storage because it
is easier to automate, requires less hardware, and is more
representative of how the device will actually be used; for example,
hardware-specific features can be tested.
