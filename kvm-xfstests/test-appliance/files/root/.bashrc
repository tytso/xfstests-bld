PATH=/root/xfstests/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
if [ "$PS1" ]; then
    # In case of an interactive shell
    . $HOME/test-env
    /root/xfstests/bin/resize
    /root/xfstests/bin/setup-fstab
    dmesg -n 8
fi
