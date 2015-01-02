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

PATH="/root/xfstests/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

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

if test "$CMD" = "ver"
then
	/usr/local/sbin/ver
	poweroff -f > /dev/null 2>&1
fi

if test -n "$FSTESTCFG" -a -n "$FSTESTSET"
then
	/root/runtests.sh
	poweroff -f > /dev/null 2>&1
fi
