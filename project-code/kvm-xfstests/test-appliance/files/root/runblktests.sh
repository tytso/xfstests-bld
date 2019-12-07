#!/bin/bash

API_MAJOR=1
API_MINOR=5
. /root/test-config
. /root/runtests_utils

RESULTS=/results
RUNSTATS="$RESULTS/run-stats"

function gce_run_hooks()
{
    if test -n "$RUN_ON_GCE"
    then
	run_hooks "$@"
    fi
}

while [ "$1" != "" ]; do
    case $1 in
	--run-once)
	    RUN_ONCE=yes
	    ;;
	*)
	    echo "Illegal option: $1"
	    exit 1
	    ;;
    esac
    shift
done

if test -z "$FSTESTAPI" ; then
    echo "Missing TEST API!"
    umount "$RESULTS"
    poweroff -f > /dev/null 2>&1
fi

set $FSTESTAPI

if test "$1" -ne "$API_MAJOR" ; then
    echo " "
    echo "API version of kvm-xfstests is $1.$2"
    echo "Major version number must be $API_MAJOR"
    echo " "
    umount "$RESULTS"
    poweroff -f > /dev/null 2>&1
fi

if test "$2" -gt "$API_MINOR" ; then
    echo " "
    echo "API version of kvm-xfstests is $1.$2"
    echo "Minor version number is greater than $API_MINOR"
    echo "Some kvm-xfstests options may not work correctly."
    echo "please update or rebuild your root_fs.img"
    echo " "
    sleep 5
fi

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

CPUS=$(cat /proc/cpuinfo  | grep ^processor | tail -n 1 | awk '{print $3 + 1}')
MEM=$(grep MemTotal /proc/meminfo | awk '{print $2 / 1024}')

if test -n "$RUN_ONCE" -a -f "$RUNSTATS"
then
    mv "$RUNSTATS" "$RUNSTATS.old"
    RESTARTED=yes
fi

cp /dev/null "$RUNSTATS"
if test -f /var/www/cmdline
then
    echo "CMDLINE: $(cat /var/www/cmdline)" >> "$RUNSTATS"
fi
if test -n "$RUN_ON_GCE"
then
    cp /usr/local/lib/gce-local.config /root/blktests/config
    echo "TEST_DEVS=(/dev/sdb)" >> /root/blktests/config
    . /usr/local/lib/gce-funcs
    image=$(gcloud compute disks describe --format='value(sourceImage)' \
		--zone "$ZONE" ${instance} | \
		sed -e 's;https://www.googleapis.com/compute/v1/projects/;;' \
		    -e 's;global/images/;;')
    echo "FSTESTIMG: $image" >> "$RUNSTATS"
    echo "FSTESTPRJ: $(get_metadata_value_with_retries project-id)" >> "$RUNSTATS"
else
    echo "TEST_DEVS=( $PRI_TST_DEV )" > /root/blktests/config
fi
if test -f /tmp/exclude-tests ; then
    EXCLUDE=$(tr "\\n" " " < /tmp/exclude-tests)
    echo "EXCLUDE=( $EXCLUDE )" >> /root/blktests/config
fi
cp /root/blktests/config /results

echo -e "KERNEL: kernel\t$(uname -r -v -m)" >> "$RUNSTATS"
sed -e 's/^/FSTESTVER: /g' /root/xfstests/git-versions >> "$RUNSTATS"
echo FSTESTSET: \"$FSTESTSET\" >> "$RUNSTATS"
echo FSTESTEXC: \"$FSTESTEXC\" >> "$RUNSTATS"
echo FSTESTOPT: \"$FSTESTOPT\" >> "$RUNSTATS"
echo CPUS:      \"$CPUS\" >> "$RUNSTATS"
echo MEM:       \"$MEM\" >> "$RUNSTATS"
if test -n "$RUN_ON_GCE"
then
    DMI_MEM=$(sudo dmidecode -t memory 2> /dev/null | \
		     grep "Maximum Capacity: " | \
		     sed -e 's/.*: //')
    if test $? -eq 0
    then
	echo "DMI_MEM: $DMI_MEM (Max capacity)" >> "$RUNSTATS"
    fi
    PARAM_MEM=$(gce_attribute mem)
    if test -n "$PARAM_MEM"
    then
	echo "PARAM_MEM: $PARAM_MEM (restricted by cmdline)" >> "$RUNSTATS"
    fi
    GCE_ID=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/id" -H "Metadata-Flavor: Google" 2> /dev/null)
    echo GCE ID:    \"$GCE_ID\" >> "$RUNSTATS"
    echo TESTRUNID: $TESTRUNID >> "$RUNSTATS"
fi

if test -z "$RUN_ON_GCE"
then
    for i in $(find "$RESULTS" -name results-\* -type d)
    do
	find $i/* -type d -print | xargs rm -rf 2> /dev/null
	find $i -type f ! -name check.time -print | xargs rm -f 2> /dev/null
    done
fi

if test -z "$RUN_ONCE"
then
    for i in $(find "$RESULTS" -name results-\* -type d)
    do
	find $i/* -type d -print | xargs rm -rf 2> /dev/null
	find $i -type f ! -name check.time -print | xargs rm -f 2> /dev/null
    done
fi

if test -z "$RESTARTED"
then
    cat "$RUNSTATS"
    free -m
else
    test -f "$RESULTS/slabinfo.before" && \
	mv "$RESULTS/slabinfo.before" "$RESULTS/slabinfo.before.old"
    test -f "$RESULTS/meminfo.before" && \
	mv "$RESULTS/meminfo.before" "$RESULTS/meminfo.before.old"
fi

touch "$RESULTS/fstest-completed"

if test ! -f /.dockerenv ; then
    echo 3 > /proc/sys/vm/drop_caches
fi
[ -e /proc/slabinfo ] && cp /proc/slabinfo "$RESULTS/slabinfo.before"
cp /proc/meminfo "$RESULTS/meminfo.before"

if test -n "$FSTESTSTR" ; then
    systemctl start stress
fi

cp config "$RESULTS/config"
echo -n "BEGIN BLKTESTS " ; date
logger "BEGIN BLKTESTS"

./check --output "$RESULTS" $FSTESTSET

free -m

echo -n "END BLKTESTS " ; date
logger "END TEST $i: $TESTNAME "

if test -n "$FSTESTSTR" ; then
    [ -e /proc/slabinfo ] && cp /proc/slabinfo "$RESULTS/slabinfo.stress"
    cp /proc/meminfo "$RESULTS/meminfo.stress"
    systemctl status stress
    systemctl stop stress
fi

if test ! -f /.dockerenv ; then
    echo 3 > /proc/sys/vm/drop_caches
fi
[ -e /proc/slabinfo ] && cp /proc/slabinfo "$RESULTS/slabinfo.after"
cp /proc/meminfo "$RESULTS/meminfo.after"

if test -n "$FSTEST_ARCHIVE"; then
    tar -C $RESULTS -cf - . | \
	xz -6e > /tmp/results.tar.xz
fi
