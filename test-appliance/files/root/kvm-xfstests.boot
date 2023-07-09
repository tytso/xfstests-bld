#!/bin/bash -e
#
# This script is executed at the end of each multiuser runlevel
# to kick off the test appliance commands

# This script will also boot the test appliance in GCE into LTM or KCS mode.

parse() {
if grep -q " $1=" /proc/cmdline; then
   cat /proc/cmdline | sed -e "s/.* $1=//" | sed -e 's/ .*//'
else
   echo ""
fi
}

ldconfig

. /root/test-config

FSTESTCFG=$(parse fstestcfg | sed -e 's/,/ /g')
FSTESTSET=$(parse fstestset | sed -e 's/,/ /g')
FSTESTOPT=$(parse fstestopt | sed -e 's/,/ /g')
FSTESTTYP=$(parse fstesttyp)
FSTESTAPI=$(parse fstestapi | sed -e 's/\./ /g')
FSTESTSTR=$(parse fsteststr | sed -e 's/\,/ /g')
ORIG_CMDLINE=$(parse orig_cmdline)
timezone=$(parse fstesttz)
MNTOPTS=$(parse mount_opts)
DISK_SPEC=$(parse disk_spec)
PTS_SIZE=$(parse pts_size)
CMD=$(parse cmd)
FSTESTEXC=$(parse fstestexc | sed -e 's/\./ /g')
FSTEST_ARCHIVE=$(parse fstestarc | sed -e 's/\./ /g')
NFSSRV=$(parse nfssrv)

cat > /run/test-env <<EOF
FSTESTCFG="$FSTESTCFG"
FSTESTSET="$FSTESTSET"
FSTESTOPT="$FSTESTOPT"
FSTESTTYP="$FSTESTTYP"
FSTESTAPI="$FSTESTAPI"
FSTESTSTR="$FSTESTSTR"
ORIG_CMDLINE="$ORIG_CMDLINE"
timezone="$timezone"
MNTOPTS="$MNTOPTS"
DISK_SPEC="$DISK_SPEC"
PTS_SIZE="$PTS_SIZE"
CMD="$CMD"
FSTESTEXC="$FSTESTEXC"
NFSSRV="$NFSSRV"
EOF

if test -e /usr/local/lib/gce-load-kernel
then
    . /usr/local/lib/gce-funcs

    if gce_attribute gce_xfs_ltm
    then
	script -a -c /usr/local/lib/gce-ltm.boot /var/log/gce-ltm-boot.log
	exit $?
    fi

    if gce_attribute gce_xfs_kcs
    then
	script -a -c /usr/local/lib/gce-kcs.boot /var/log/gce-kcs-boot.log
	exit $?
    fi

    if ! grep -q fstestcfg /proc/cmdline
    then
	script -a -f -c /usr/local/lib/gce-load-kernel \
	       /var/log/gce-load-kernel.log
    fi
    . /run/test-env
    # for interactive mounting using the fstab entries
    ln -s "$PRI_TST_DEV" /dev/vdb
    ln -s "$SM_SCR_DEV" /dev/vdc
    ln -s "$SM_TST_DEV" /dev/vdd
    ln -s "$LG_SCR_DEV" /dev/vde
    ln -s "$LG_TST_DEV" /dev/vdf
    systemctl start lighttpd.service
elif test -b /dev/vdh
then
    mkdir /tmp/upload
    tar -C /tmp/upload -xf /dev/vdh
    if test -f /tmp/upload/xfstests.tar.gz ; then
	rm -rf /root/xfstests
	tar -C /root -xzf /tmp/upload/xfstests.tar.gz
	rm -f /tmp/upload/xfstests.tar.gz
    fi
    if test -f /tmp/upload/extra-tests.tar.gz ; then
	tar -C /root -xzf /tmp/upload/extra-tests.tar.gz
	rm -f /tmp/upload/extra-tests.tar.gz
    fi
    if test -f /tmp/upload/files.tar.gz ; then
	tar -C / -xzf /tmp/upload/files.tar.gz
	rm -f /tmp/upload/files.tar.gz
    fi
    if test -f /tmp/upload/modules.tar.xz ; then
       tar -C / -xJf /tmp/upload/modules.tar.xz
       rm -f /tmp/upload/modules.tar.xz
       depmod -a
    fi
fi

export FSTESTCFG FSTESTSET FSTESTOPT FSTESTTYP FSTESTAPI FSTESTSTR FSTESTEXC
export MNTOPTS FSTEST_ARCHIVE NFSSRV FILESTORE_TOP FILESTORE_SUBDIR
export ORIG_CMDLINE

case "$FSTESTOPT" in
    *blktests*)
	export DO_BLKTESTS=yes
	touch /run/do_blktests
	;;
esac

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

if test "$CMD" = "pts"
then
    if test -n "$RUN_ON_GCE"
    then
	script -a -c /usr/local/lib/gce-setup /var/log/gce-setup.log
	. /run/test-env
	exit 0
	/root/run-pts.sh >> /results/runtests.log 2>&1

	/usr/local/lib/gce-logger tests complete
	/bin/rm -f /run/gce-finalize-wait
    else
	# Not yet supported on KVM...
	/root/run-pts.sh
	if test -b /dev/vdh -a -n "$FSTEST_ARCHIVE"
	then
	    tar -C /tmp -cf /dev/vdh results.tar.xz
	fi
	umount /results
	poweroff -f > /dev/null 2>&1
    fi
fi

if test -n "$DO_BLKTESTS"
then
    if test -n "$RUN_ON_GCE"
    then
	/usr/local/lib/gce-setup
	. /run/test-env
	/root/runblktests.sh --run-once >> /results/runtests.log 2>&1

	/usr/local/lib/gce-logger tests complete
	script -c "/bin/bash -vx /usr/local/sbin/gce-shutdown" /dev/console
	/bin/rm -f /run/gce-finalize-wait
    else
	/root/runblktests.sh
	if test -b /dev/vdh -a -n "$FSTEST_ARCHIVE"
	then
	    tar -C /tmp -cf /dev/vdh results.tar.xz
	fi
	umount /results
	poweroff -f > /dev/null 2>&1
    fi
elif test -n "$FSTESTCFG" -a -n "$FSTESTSET"
then
    if test -n "$RUN_ON_GCE"
    then
	/usr/local/lib/gce-setup
	. /run/test-env
	/root/runtests.sh --run-once >> /results/runtests.log 2>&1

	/usr/local/lib/gce-logger tests complete
	/bin/rm -f /run/gce-finalize-wait
    else
	/root/runtests.sh
	if test -b /dev/vdh -a -n "$FSTEST_ARCHIVE"
	then
	    tar -C /tmp -cf /dev/vdh results.tar.xz
	fi
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
	    . /run/test-env
	    /usr/local/lib/gce-logger setup complete
	else
	    /usr/local/lib/gce-add-metadata "kernel_version=$(uname -a)" &
	fi
	if test -n "$(gce_attribute no_vm_timeout)" ; then
	    systemctl stop gce-finalize.timer
	    systemctl disable gce-finalize.timer
	    logger -i "Disabled gce-finalize timer"
	fi
    fi
fi
