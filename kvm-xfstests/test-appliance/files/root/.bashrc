PATH=/root/xfstests/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
if [ "$PS1" ]; then
    # In case of an interactive shell
    . $HOME/test-config
    . $HOME/test-env
    /root/xfstests/bin/resize
    if test -z "$RUN_ON_GCE"
    then
	/root/xfstests/bin/setup-fstab
    fi
    dmesg -n 8
fi
