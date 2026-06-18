# test-appliance

This directory contains scripts and configurations to create the
test appliances, or the operating system environment where filesystem
tests are actually run.

For QEMU/KVM and Android test appliances, the scripts builds appliance
on top of a minimal debian system built using a modified script based
on For the Google Compute Engine (GCE) test appliance, the
gce-create-image script create a builder instance (VM) using the
standard GCE Debian image, and then runs gce-xfstests-bld.sh script in
the VM to modify the root file system to create the test appliance.
Once the root file system is fully customized, and the builder
instance is completed and deleted, the gce-create-image script will
complete the process by creating a gce-xfstests test appliance image
from the builder instance's Persistent Disk.

The test appliance has three components:

*  The Debian system files, either created by the modified debootstrap script,
   named gen-image, for QEMU/KVM and Android test appliances, or from
   the GCE's official debianm image.  Some additional Debian packages will
   also be installed, either by gen-image or gce-xfstests-bld.sh script.

*  The directory hierarchy rooted at test-appliance/files is layered
   onto test appliance.  The bulk of the control scripts and programs that are
   unique to this test appliance are found here.

*  The xfstests.tar.gz file which is built in the top-level fstests-bld
   directory and which containing the xfstests test suite and its
   dependencies is unpacked into the /root/xfstests directory.
