#!/bin/bash -e
#
# This script is executed at the end of each multiuser runlevel
# to kick off the test appliance commands

# This script will also boot the test appliance in GCE into LTM mode.

parse() {
if grep -q " $1=" /proc/cmdline; then
   cat /proc/cmdline | sed -e "s/.* $1=//" | sed -e 's/ .*//'
else
   echo ""
fi
}

. /root/test-config

FSTESTCFG=$(parse fstestcfg | sed -e 's/,/ /g')
FSTESTSET=$(parse fstestset | sed -e 's/,/ /g')
FSTESTOPT=$(parse fstestopt | sed -e 's/,/ /g')
FSTESTTYP=$(parse fstesttyp)
FSTESTAPI=$(parse fstestapi | sed -e 's/\./ /g')
timezone=$(parse fstesttz)
MNTOPTS=$(parse mount_opts)
CMD=$(parse cmd)
FSTESTEXC=$(parse fstestexc | sed -e 's/\./ /g')
FSTEST_ARCHIVE=$(parse fstestarc | sed -e 's/\./ /g')

cat > /run/test-env <<EOF
FSTESTCFG="$FSTESTCFG"
FSTESTSET="$FSTESTSET"
FSTESTOPT="$FSTESTOPT"
FSTESTTYP="$FSTESTTYP"
FSTESTAPI="$FSTESTAPI"
timezone="$timezone"
MNTOPTS="$MNTOPTS"
CMD="$CMD"
FSTESTEXC="$FSTESTEXC"
EOF

if test -e /usr/local/lib/gce-kexec
then
    . /usr/local/lib/gce-funcs

    if gce_attribute gce_xfs_ltm
    then
	/usr/local/lib/gce-ltm.boot
	exit $?
    fi

    /usr/local/lib/gce-kexec
    . /run/test-env
    # for interactive mounting using the fstab entries
    ln -s "$PRI_TST_DEV" /dev/vdb
    ln -s "$SM_SCR_DEV" /dev/vdc
    ln -s "$SM_TST_DEV" /dev/vdd
    ln -s "$LG_SCR_DEV" /dev/vde
    ln -s "$LG_TST_DEV" /dev/vdf
elif test -b /dev/vdh
then
    mkdir /tmp/upload
    tar -C /tmp/upload -xf /dev/vdh
    if test -f /tmp/upload/xfstests.tar.gz; then
	rm -rf /root/xfstests
	tar -C /root -xzf /tmp/upload/xfstests.tar.gz
	rm -f /tmp/upload/xfstests.tar.gz
    fi
    if test -f /tmp/upload/files.tar.gz; then
	tar -C / -xzf /tmp/upload/files.tar.gz
	rm -f /tmp/upload/files.tar.gz
    fi
fi

export FSTESTCFG
export FSTESTSET
export FSTESTOPT
export FSTESTTYP
export FSTESTAPI
export FSTESTEXC
export MNTOPTS
export FSTEST_ARCHIVE

if test -n "$timezone" -a -f /usr/share/zoneinfo/$timezone
then
    ln -sf /usr/share/zoneinfo/$timezone /etc/localtime
    echo $timezone > /etc/timezone
fi

# work around a bug which causes LVM to fail on older kernels
if test -f /sys/kernel/uevent_helper
then
    echo > /sys/kernel/uevent_helper
fi

if test "$CMD" = "ver"
then
	/usr/local/sbin/ver
	poweroff -f > /dev/null 2>&1
fi

if test -n "$FSTESTCFG" -a -n "$FSTESTSET"
then
    if test -n "$RUN_ON_GCE"
    then
	/usr/local/lib/gce-setup
	/root/runtests.sh --run-once >> /results/runtests.log 2>&1

	/usr/local/lib/gce-logger tests complete
	/bin/rm -f /run/gce-finalize-wait
    else
	/root/runtests.sh
	umount /results
	poweroff -f > /dev/null 2>&1
    fi
else
    if test -n "$RUN_ON_GCE"
    then
	if test -n "$(gce_attribute kexec)"
	then
	    # If we kexec'ed into a test kernel, we probably want to
	    # run tests, so set up the scratch volumes
	    /usr/local/lib/gce-setup
	    /usr/local/lib/gce-logger setup complete
	else
	    /usr/local/lib/gce-add-metadata "kernel_version=$(uname -a)" &
	fi
    fi
fi
