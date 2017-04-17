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
  xfstests-bld, then move android-xfstests.sh to
  ~/bin/android-xfstests or another location on your $PATH.

- A rooted Android device with sufficient internal storage.  For most
  test configurations, about 24 GiB of internal storage should be
  sufficient.  This is the sum of three 5 GiB partitions, a shrunken 4
  GiB userdata partition, and the various other partitions used by
  Android devices.  For test configurations requiring large
  partitions, like bigalloc, you'll need about 64 GiB instead.

- Ability to connect to the Android device with adb and fastboot.
  Usually this is done via a USB cable.

- An armhf Debian root filesystem set up with xfstests and the
  xfstests-bld scripts.  Either fetch the prebuilt
  armhf_root_fs.tar.gz from
  [kernel.org](http://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests),
  or build one yourself on a Debian ARM build server as described in
  [building-xfstests](building-xfstests.md).  Then, either put the
  chroot tarball in the default location of
  kvm-xfstests/test-appliance/armhf_root_fs.tar.gz, or specify it with
  ROOT_FS in your ~/.config/android-xfstests or the -I option to
  android-xfstests.

## Procedure

### (Optional) Build a custom kernel

You may be able to run xfstests with the kernel already installed on
your device, but you may wish to build your own kernel instead.  The
exact procedure for building the kernel is device-dependent, but here
are example commands for building a kernel using the public source
code for the Google Pixel phone (2016 edition), code name "marlin":

    git clone https://android.googlesource.com/kernel/msm msm-linux
    cd msm-linux
    git checkout android-msm-marlin-3.18-nougat-mr1
    export CROSS_COMPILE=aarch64-linux-android-
    export ARCH=arm64
    make marlin_defconfig
    make -j$(grep -c processor /proc/cpuinfo)

This will produce a kernel image arch/arm64/boot/Image.gz-dtb.

Also consider the following config options:

    CONFIG_SYSV_IPC=y
        Makes some tests using dm-setup stop failing.

    CONFIG_CRYPTO_MANAGER_DISABLE_TESTS=n
        Include crypto self-tests.  This may be useful if you are
        using xfstests to test file-based encryption.

### (Optional) Boot into your custom kernel

To boot into your new kernel, you'll first need to reboot your device
into fastboot mode by running 'adb reboot-bootloader' or by holding a
device-dependent key combination (e.g. Power + Vol-Down on the Pixel).
Then do *one* of the following:

- Run 'fastboot boot arch/arm64/boot/Image.gz-dtb' to boot the kernel
  directly.  Careful: this is good for one boot only (it's not
  persistent), and it doesn't work on all devices.

- Build and flash a boot.img to your device's boot partition.  This is
  device-dependent, but for "marlin" devices one would copy
  arch/arm64/boot/Image.gz-dtb into device/google/marlin-kernel/ in
  the Android source tree, then run the following commands from the
  root of the Android source tree:

    . build/envsetup.sh
    lunch marlin-userdebug
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

## Known issues

android-xfstests doesn't yet do kernel installation; you have to do
that yourself.

Terminating android-xfstests with Ctrl-C doesn't stop the test process
on the device.

'android-xfstests shell' gives you a shell in the chroot, but it's not
a snapshot like it is for kvm-xfstests; that is, any changes you make
in the shell session are persistent.

Android devices usually run an older version of the Linux kernel.  At
the same time, xfstests is constantly being updated to add new tests.
Therefore, you can expect there to be a significant number of failing
tests due to bugs.  Some tests may even cause a kernel crash or
deadlock and will need to be excluded with -X in order for the test
run to complete.  Note, however, that bugs reproduced by xfstests are
not necessarily reachable by unprivileged users (though they can be!).

Tests which create loopback or device-mapper devices currently fail
because the corresponding device nodes do not get automatically
created on Android.

Any test that requires non-root users currently fails because xfstests
incorrectly thinks that YP/NIS is enabled.

On recent versions of Android, all new files inherit SELinux xattrs.
This confuses generic/062 and generic/377 and causes them to fail.

generic/240 and tests using dmsetup fail on kernels configured without
SysV IPC support, which includes most Android kernels.

generic/004 fails because of glibc bug
https://sourceware.org/bugzilla/show_bug.cgi?id=17912.

## Resetting userdata

The partitions set up by android-xfstests are transient, so they will
not show up in the device's on-disk partition table or fstab, and they
will go away after reboot.  However, the reformatting of the userdata
filesystem with a smaller size to free up space is persistent.  If you
are done running xfstests and wish to expand userdata to take up its
full partition again, then reboot into fastboot mode and run 'fastboot
-w' to wipe and reformat userdata again, this time with the full size.

## Other notes

Note that xfstests does pretty heavy I/O.  It is also possible to run
xfstests on external storage, e.g. on a USB-attached SSD.  However,
android-xfstests currently only supports internal storage because it
is easier to automate, requires less hardware, and is more
representative of how the device will actually be used; for example,
hardware-specific features can be tested.
