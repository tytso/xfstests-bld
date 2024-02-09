#!/bin/bash

API_MAJOR=1
API_MINOR=5
. /root/test-config
. /root/runtests_utils

RESULTS=/results
RUNSTATS="$RESULTS/run-stats"

runtests_setup

if test -n "$FSTESTOPT" ; then
	set $FSTESTOPT
else
	set ""
fi

RPT_COUNT=1

while [ "$1" != "" ]; do
  case "$1" in
    aex)
	DO_AEX=t
	;;
    ltptests)
	;;
    count) shift
	RPT_COUNT=$1
	;;
    extra_opt) shift
	EXTRA_OPT="$EXTRA_OPT $1"
	;;
    *)
	echo " "
	echo "Unrecognized option $1"
	echo " "
  esac
  shift
done

if test ! -f /.dockerenv ; then
    dmesg -n 5
fi
cd /root/ltp

if test -n "$FSTESTEXC" ; then
	echo $FSTESTEXC | tr , \\n > /tmp/exclude-tests
else
	rm -f /tmp/exclude-tests
fi
if test -n "$DO_AEX" ; then
    if test -f "/root/ltptests.exclude" ; then
	sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
	    < "/root/ltptests.exclude" >> /tmp/exclude-tests
    fi
fi

if test -f /tmp/exclude-tests ; then
    EXCLUDE=$(tr "\\n" " " < /tmp/exclude-tests)
    echo "EXCLUDE=( $EXCLUDE )" >> /root/ltp/config
fi
cp /root/ltp/config /results

touch "$RESULTS/fstest-completed"

runtests_before_tests

mkdir -p /results/ltp /root/ltp/results
set -vx
mount --bind /results/ltp /root/ltp/results 

cp config "$RESULTS/config"
echo -n "BEGIN LTP " ; date
logger "BEGIN LTP"

./runltp

free -m

echo -n "END LTP " ; date
logger "END TEST $i: $TESTNAME "

umount /root/ltp/results

runtests_after_tests

runtests_save_results_tar
