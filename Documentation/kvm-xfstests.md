# kvm-xfstests: Running XFStests using virtualization

Please read the [kvm-quickstart](kvm-quickstart.md) instructions
first, since this will allow you to get started quickly.

If you don't have any familiarity with xfstests, you may also want to
read this [introduction to xfstests](what-is-xfstests.md).

## Installation

The kvm-xfstests system consists of a series of shell scripts, and a
test appliance virtual machine image.  You can build an image using
the build infrastructure in the xfstests-bld git repository, but if
you are just getting started, it will be much simpler if you download
one of the pre-compiled VM images which can be found on
[kernel.org](https://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests).

You will find there a 32-bit test appliance named
[root_fs.img.i386](https://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests/root_fs.img.i386)
and a 64-bit test appliance named
[root_fs.img.amd64](https://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests/root_fs.img.amd64).
This file should be installed as root_fs.img in the
kvm-xfstests/test-appliance directory.

A 64-bit x86 kernel can use both the 32-bit and 64-bit test appliance
VM, since you can run 32-bit ELF binaries using a 64-bit kernel.
However, the reverse is not true; a 32-bit x86 kernel can not run
64-bit x86 binaries.  This makes the 64-bit test appliance more
flexible.  In addition, if you use the 64-bit kernel with 32-bit
interfaces, it tests the 32-bit compat ioctl code paths, which
otherwise may not get sufficient testing.

If you want to build your own test appliance VM, see
[building-rootfs.md](building-rootfs.md).

## Setup and configuration

The configuration file for kvm-xfstests is found in the kvm-xfstests
directory and is named config.kvm.  You can edit this file directly,
but the better thing to do is to place override values in
~/.config/kvm-xfstests.  Please look at the kvm-xfstests/config.kvm
file to see the shell variables you can set.

Perhaps the most important configuration variable to set is KERNEL.
This should point at the default location for the kernel that qemu
will boot to run the test appliance.  This is, in general, should be
the primary build tree that you use for kernel development.  If
kvm-xfstests is run from the top-level of a kernel build or source
tree where there is a built kernel, kvm-xfstests will use it.
Otherwise, it will use the kernel specified by the KERNEL variable.

The kernel for kvm-xfstests must not use modules, and it must include
the paravirtual device drivers needed for qemu.  To build a correctly
configured kernel, base your configuration on one of the files in the
kernel-configs directory.  That is, copy the config for the desired
architecture and kernel version (or the closest available version) to
.config in your kernel build tree, then run 'make olddefconfig' ('make
oldnoconfig' for pre-3.7 kernels).  This can be automated via the
command:

        kvm-xfstests install-kconfig [--i386]

(Add the --i386 option if you wish to build a 32-bit kernel.)

By default, the scratch disks used by test-appliance will be set up
automatically, and are stored in the kvm-xfstests directory with the
names vdb, vdc, vdd, ... up to vdg.  However, it is slightly faster to
use logical volumes.  To do this override the VDB..VDG variables:

        VG=closure

        VDB=/dev/$VG/test-4k
        VDC=/dev/$VG/scratch
        VDD=/dev/$VG/test-1k
        VDE=/dev/$VG/scratch2
        VDF=/dev/$VG/scratch3
        VDG=/dev/$VG/results

If you chose to do this, the logical volumes for VDB, VDC, VDD, and
VDG should be 5 gigabytes, while VDE and VDF should be 20 gigabyte
logical volumes.  The devices VDB and VDG should have an ext4 file
system created using the mkfs.ext4 command before you try running kvm-xfstests.

## Running kvm-xfstests

The kvm-xfstests shell script is in the kvm-xfstests directory, and it
is designed to be run with the current working directory to be in the
kvm-xfstests directory.  For convenience's sake, the Makefile in the
top-level directory of xfstests-bld will create a kvm-xfstests.sh
shell script which can be copied into a convenient directory in your
PATH.  This shell script will set the KVM_XFSTESTS_DIR environment
variable so the auxiliary files can be found and then runs the
kvm-xfstests/kvm-xfstests shell script.

Please run "kvm-xfstests help" to get a quick summary of the available
command-line syntax.  Not all of the available command-line options
are documented; some of the more specialized options will require that
you Read The Fine Source --- in particular, in the auxiliary script
file found in kvm-xfstests/util/parse_cli.

### Running file system tests

The general form of the kvm-xfstests command to run tests in the test
appliance is:

        kvm-xfstests [-c <cfg>] [-g <group>]|[<tests>] ...


By default <cfg> defaults to all, which will run the following
configurations: "4k", "1k", "ext3", "nojournal", "ext3conv",
"dioread_nolock, "data_journal", "inline", "bigalloc", and
"bigalloc_1k".  You may specify a single configuration or a comma
separated list if you want to run a subset of all possible file system
configurations.

Tests can be specified using an xfstests group via "-g <group>", or
via one or more specific xfstests subtests (e.g., "generic/068").  The
most common test groups you will use are "auto" which runs all of the
tests that are suitable for use in an automated test run, and "quick"
which runs a subset of the tests designed for a fast smoke test.

For developer convenience, "kvm-xfstests smoke" is short-hand for
"kvm-xfstests -c 4k -g quick", which runs the fast subset of tests
using just 4k block file system configuration.  In addition
"kvm-xfstests full" is short-hand for "kvm-xfstests -g auto" which
runs all of the tests using a large set of file system configurations.
This will take quite a while, so it's best run overnight.  (Or it may
be better to run the full set of tests using gce-xfstests.)

### Running an interactive shell

The command "kvm-xfstests shell" will allow you to examine the tests
environment or to run tests manually, by booting the test kernel and
requesting that the test appliance VM start an interactive shell.

Any changes to the root partition will be reverted when you exit the
VM.  If you would like to modify the root_fs.img appliance
permanently, you can run "kvm-xfstests maint" instead.

You can run tests manually by looking at the environment variables set
in the /root/test-env file (which is sourced automatically when you
start an interactive shell).  You can then set FSTESTCFG and FSTESTSET
to control which tests you would like to run, and then run the test
runner script, /root/runtests.sh.  For example:

        % kvm-xfstests shell
        # FSTESTCFG="4k encrypt"
        # FSTESTSET="generic/001 generic/002 ext4/001"
        # /root/runtests.sh
        ...

To stop the VM, you can run the "poweroff" command, but a much faster way
to shut down the VM is to use the command sequence "C-a x" (that is,
Control-a followed by the character 'x'). 

## Local debugging ports

While kvm-xfstests is running, you can telnet to a number of TCP ports
(which are bound to localhost).  Ports 7500, 7501, and 7502 will
connect you to a shell prompts while the tests are running (if you
want to check on /proc/slabinfo, enable tracing, etc.)  You can also
use these ports in conjunction with "kvm-xfstests shell" if you want
additional windows to capture traces using ftrace.

You can also access the qemu monitor on port 7498, and you can debug the
kernel using remote gdb on localhost port 7499.  Just run "gdb
/path/to/vmlinux", and then use the command "target remote
localhost:7499".

Pro tips for using remote gdb: it's helpful to temporarily add
"EXTRA_CFLAGS += -O0" to fs/{ext4,jbd2}/Makefile, and use a kernel
config with debug features enabled via "kvm-xfstests install-kconfig
--debug".  In addition, you may need to add to your $HOME/.gdbinit the
line "add-auto-load-safe-path /path/to", where /path/to is the
directory containing the compiled vmlinux executable.  See
[Documentation/dev-tools/gdb-kernel-debugging.rst](https://www.kernel.org/doc/html/latest/dev-tools/gdb-kernel-debugging.html)
in the kernel sources for more information.

## Log files

By default, when test results are saved in the kvm-xfstests directory
with the filename log.<DATECODE>.

The get-results command will summarize the output from the log file.
It takes as an argument the name of the log file; if no log file is
specified, then the get-results command will display a summary of the
most recent log file.
