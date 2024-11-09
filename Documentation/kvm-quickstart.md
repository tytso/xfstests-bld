# Quick start instructions for kvm-xfstests

1.  Make sure the necessary packages are installed.  For Debian/Ubuntu
    systems:

        apt-get install qemu-kvm wget gcc git make

    For Fedora systems:

        dnf install qemu-kvm wget2 gcc git make

    For openSUSE Tumbleweed:

        zypper in qemu wget2 gcc git make

2.  Run the following commands to install the xfstests-bld repository
    and install the necessary scripts into the bin directory in your
    home directory.  If ~/bin isn't in your PATH, edit your dotfiles
    (e.g., your ~/.bashrc) so that it is.

        git clone https://github.com/tytso/xfstests-bld fstests
        cd fstests
        make ; make install

3.  Optionally, if you want to primarily developing a file system
    other than ext4, you can specify the primary file system type in the
    file ~/.config/kvm-xfstests:

        echo PRIMARY_FSTYPE=f2fs >> ~/.config/kvm-xfstests

    Again, optionally, if you want the log files to display times in
    your local timezone, you can add a timezone to the
    ~/.config/kvm-xfstests file.

        echo TZ=America/New_York >> ~/.config/kvm-xfstests

4.  To build a kernel for use with kvm-xfstests, with the current
    directory in the kernel sources which you would like to use, run
    the commands:

        install-kconfig
        kbuild

5.  In the top-level of the kernel sources where you have run "kbuild"
    you can perform a smoke test:

        kvm-xfstests smoke

    Developers are *strongly* recommended to run a smoke test before
    submitting a patch or patch series upstream for review.

    To do a full test, you can run "kvm-xfstests full".   Warning:
    this will take a long time --- close to 24 hours if you are
    testing ext4; you may be better off using
    [gce-xfstests](gce-xfstests.md) if you are interested in doing the sort of
    testing used by file system maintainers.

For more information, please see the full [kvm-xfstests
documentation](kvm-xfstests.md).
