#!/bin/bash -e
#
# This script is executed at the end of each multiuser runlevel
# to kick off the test appliance commands

parse() {
if grep -q " $1=" /proc/cmdline; then
   cat /proc/cmdline | sed -e "s/.* $1=//" | sed -e 's/ .*//'
else
   echo ""
fi
}

. /root/test-config

if test -e /usr/local/lib/gce-kexec
then
    . /usr/local/lib/gce-funcs
    /usr/local/lib/gce-kexec
fi

FSTESTCFG=$(parse fstestcfg | sed -e 's/,/ /g')
FSTESTSET=$(parse fstestset | sed -e 's/,/ /g')
FSTESTOPT=$(parse fstestopt | sed -e 's/,/ /g')
FSTESTTYP=$(parse fstesttyp)
FSTESTAPI=$(parse fstestapi | sed -e 's/\./ /g')
timezone=$(parse fstesttz)
MNTOPTS=$(parse mount_opts)
CMD=$(parse cmd)
FSTESTEXC=$(parse fstestexc | sed -e 's/\./ /g')

export FSTESTCFG
export FSTESTSET
export FSTESTOPT
export FSTESTTYP
export FSTESTAPI
export FSTESTEXC
export MNTOPTS

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
	/root/runtests.sh >& /results/runtests.log

	/usr/local/lib/gce-logger tests complete
	gce-shutdown
    else
	/root/runtests.sh
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
	    gcloud compute -q instances set-disk-auto-delete ${instance} \
		   --auto-delete --disk "$instance-results" --zone "$ZONE" &
	fi
    fi
fi
