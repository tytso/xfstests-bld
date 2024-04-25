#!/bin/bash

API_MAJOR=1
API_MINOR=5
. /root/test-config
. /root/runtests_utils

runtests_setup "$@"

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
    blktests)
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
cd /root/blktests

if test -n "$FSTESTEXC" ; then
	echo $FSTESTEXC | tr , \\n > /tmp/exclude-tests
else
	rm -f /tmp/exclude-tests
fi
if test -n "$DO_AEX" ; then
    if test -f "/root/blktests.exclude" ; then
	sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
	    < "/root/blktests.exclude" >> /tmp/exclude-tests
    fi
fi

if test -n "$RUN_ON_GCE"
then
    cp /usr/local/lib/gce-local.config /root/blktests/config
    echo "TEST_DEVS=(/dev/sdb)" >> /root/blktests/config
else
    echo "TEST_DEVS=( $PRI_TST_DEV )" > /root/blktests/config
fi
if test -f /tmp/exclude-tests ; then
    EXCLUDE=$(tr "\\n" " " < /tmp/exclude-tests)
    echo "EXCLUDE=( $EXCLUDE )" >> /root/blktests/config
fi
cp /root/blktests/config /results

touch "$RESULTS/fstest-completed"

runtests_before_tests

cp config "$RESULTS/config"
echo -n "BEGIN BLKTESTS " ; date
logger "BEGIN BLKTESTS"

./check --output "$RESULTS" $FSTESTSET

free -m

echo -n "END BLKTESTS " ; date
logger "END TEST $i: $TESTNAME "

runtests_after_tests

runtests_save_results_tar
