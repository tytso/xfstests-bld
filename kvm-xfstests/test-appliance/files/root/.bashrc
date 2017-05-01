PATH=/root/xfstests/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
if [ "$PS1" ]; then
    # In case of an interactive shell
    . $HOME/test-config
    . $HOME/test-env
    /root/xfstests/bin/resize
    if [ ! -e /dev/vdb ]; then
	# for interactive mounting using the fstab entries
	ln -s $PRI_TST_DEV /dev/vdb
	ln -s $SM_SCR_DEV /dev/vdc
	ln -s $SM_TST_DEV /dev/vdd
	[ -n "$LG_SCR_DEV" ] && ln -s $LG_SCR_DEV /dev/vde
	[ -n "$LG_TST_DEV" ] && ln -s $LG_TST_DEV /dev/vdf
    fi
    dmesg -n 8
fi
