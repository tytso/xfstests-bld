# Quick start instructions for kvm-xfstests

1.  Make sure qemu with kvm support is installed on your system.

        apt-get install qemu-kvm

2.  Run the following commands to install the xfstests-bld repository
    and download a pre-compiled test appliance image.  We use the
    32-bit test appliance here since it can support both 32-bit and
    64-bit kernels.

        git clone git://git.kernel.org/pub/scm/fs/ext2/xfstests-bld.git fstests
        cd fstests
        wget -O test-appliance/root_fs.img https://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests/root_fs.img.i386

3.  In the top-level directory of your checked out xfstests-bld
    repository, run "make kvm-xfstests.sh" and then copy this
    generated file to a directory which is your shell's PATH.  This
    allows you to run the kvm-xfstests binary without needing to set
    the working directory to the kvm-xfstests directory.

        make
        cp kvm-xfstests ~/bin/kvm-xfstests

4.  In the fstests/run-fstests/ directory, take a look at the
    "config.kvm" file and either edit that file in place, or (this is
    preferred) put override values in ~/.config/kvm-xfstests.  The
    most common values you will likely need to override are the
    location of the compiled kernel and the preferred timezone if you
    wish the log files to display times in your local timezone.

        TZ=America/New_York
        KERNEL=/build/ext4/arch/x86/boot/bzImage

5.  To build a kernel for use with kvm-xfstests, with the current
    directory in the kernel sources which you would like to use, run
    the commands:

        kvm-xfstests install-kconfig
	kvm-xfstests kbuild

6.  Run "kvm-xfstests smoke" to do a quick test.  Or "kvm-xfstests
    -g auto" to do a full test.  You can also run specific tests on
    specific configurations, i.e., "kvm-xfstests -c bigalloc
    generic/013 generic/127".   To run a shell, use "kvm-xfstests shell"

For more information, please see the full [kvm-xfstests
documentation](kvm-xfstests.md).
