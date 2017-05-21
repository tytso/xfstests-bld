PATH=/root/xfstests/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
if [ "$PS1" ]; then
    # In case of an interactive shell
    . $HOME/test-config
    . $HOME/test-env
    case "$(tty)" in
	/dev/ttyS*)
	    :
	    ;;
	*)
	    /root/xfstests/bin/resize
    esac
    dmesg -n 8
fi
