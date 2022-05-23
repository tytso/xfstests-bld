export PATH=/root/xfstests/bin:/usr/local/sbin:/usr/local/bin:/usr/local/lib/go/bin:/usr/sbin:/usr/bin:/sbin:/bin

if [ "$PS1" ]; then
    # In case of an interactive shell
    . $HOME/test-config
    . $HOME/test-env
    if test -z "$RUN_ON_GCE" ; then
       # Fix line wrap from qemu
       echo -ne '\e[?7h'
       /root/xfstests/bin/resize
    else
	case "$(tty)" in
	    /dev/ttyS*)
		:
		;;
	    *)
		/root/xfstests/bin/resize
	esac
    fi
    dmesg -n 8
fi

if command -v ccache &> /dev/null /cache/ccache ; then
    export PATH="/usr/lib/ccache:$PATH"
fi
