# Running xfstests on Android

These instructions are still Alpha quality.  They haven't been
verified, although a developer has reported success following them at
least roughly.  Sorry, I haven't had a chance to verify them yet.

These instructions assume you have either fetched the
armhf_root_fs.tar.gz file from
[kernel.org](http://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests)
or you have build the armhf_root_fs.tar.gz on a Debian arm build
server as described in the [building-xfstests](building-xfstests.md)
documentation file.

1. Set up a USB attached SSD such that it has at least 5 partitions,
each of them 5GB each.  (You will need to format partitions #1, #2,
and #5 using "mke2fs -t ext4".)

  Note: you will need to adjust /root/test-config so that settings for
  VDB, VDC, VDD, and VDG point at partition #2, #3, #4, and #5
  respectively.

  * Partition #1 --- will contain the chroot directory (unpack root_fs.tar.gz)
  * Partition #2 --- will contain a "normal" formatted ext4 file system
  * Partition #3 --- will be used as a scratch partition
	(will be formatted multiple times, by individual tests)
  * Partition #4 --- will be used to test non-standard ext4 file systems
  	(such as ext4 encryption; formatted by runtests.sh)
  * Partition #5 --- will contain the test results (will be mounted on /results)

2.  Attach the SSD to the Android device with a USB C connector using a
USB C hub with power delivery.  (Anker makes a good one which is
available on Amazon.)

3. Build and install a test kernel which has SELinux in permissive mode

4. Mount the chroot partition on the USB attached SSD on /chroot, and
then set it up as follows:

        mount -t proc proc /chroot/proc
        mount -t sysfs sysfs /chroot/sys
        mount --bind /dev /chroot/dev
        mount /dev/partition#5 /chroot/results

  (note, if you don't have a mount with --bind support, you can also use
  tar to copy in a /dev into the chroot)

5. To run the tests in the chroot:

        chroot /chroot /bin/bash
        cd /root
        . test-env
        FSTESTCFG=4k,encrypt
        FSTESTSET="-g auto"
        ./runtests.sh >& /results/runtests.log

