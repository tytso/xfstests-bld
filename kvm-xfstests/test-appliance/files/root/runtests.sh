#!/bin/bash

API_MAJOR=1
API_MINOR=1
. /root/test-config

function gce_run_hooks()
{
    if test -n "$RUN_ON_GCE"
    then
	run_hooks "$@"
    fi
}

if test -z "$FSTESTAPI" ; then
    echo "Missing TEST API!"
    umount /results
    poweroff -f > /dev/null 2>&1
fi

set $FSTESTAPI

if test "$1" -ne "$API_MAJOR" ; then
    echo " "
    echo "API version of kvm-xfstests is $1.$2"
    echo "Major version number must be $API_MAJOR"
    echo " "
    umount /results
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
  case $1 in
    aex)
	DO_AEX=t
	;;
    count) shift
	RPT_COUNT=$1
	;;
    *)
	echo " "
	echo "Unrecognized option $i"
	echo " "
  esac
  shift
done

umount "$PRI_TST_DEV" >& /dev/null
umount "$SM_TST_DEV" >& /dev/null
/sbin/e2fsck -fy "$PRI_TST_DEV" >& "/tmp/fsck.$$"
FSCKCODE=$?
if test $FSCKCODE -gt 1
then
    cat /tmp/fsck.$$
    echo e2fsck failed with exit code $FSCKCODE
fi

if test $FSCKCODE -ge 8
then
	mke2fs -F -q -t ext4 $PRI_TST_DEV
fi
dmesg -n 5
cd /root/xfstests

if test "$FSTESTCFG" = all
then
	FSTESTCFG="4k 1k ext3 encrypt nojournal ext3conv dioread_nolock data_journal inline bigalloc bigalloc_1k"
fi

if test -n "$FSTESTEXC" ; then
	echo $FSTESTEXC | tr , \\n > /tmp/exclude-tests
else
	rm -f /tmp/exclude-tests
fi

CPUS=$(cat /proc/cpuinfo  | grep ^processor | tail -n 1 | awk '{print $3 + 1}')
MEM=$(grep MemTotal /proc/meminfo | awk '{print $2 / 1024}')

cp /dev/null /results/run-stats
if test -f /var/www/cmdline
then
    echo "CMDLINE: $(cat /var/www/cmdline)" >> /results/run-stats
fi
sed -e 's/^/FSTESTVER: /g' /root/xfstests/git-versions >> /results/run-stats
echo -e "FSTESTVER: kernel\t$(uname -r -v -m)" >> /results/run-stats
echo FSTESTCFG: \"$FSTESTCFG\" >> /results/run-stats
echo FSTESTSET: \"$FSTESTSET\" >> /results/run-stats
echo FSTESTEXC: \"$FSTESTEXC\" >> /results/run-stats
echo FSTESTOPT: \"$FSTESTOPT\" >> /results/run-stats
echo MNTOPTS:   \"$MNTOPTS\" >> /results/run-stats
echo CPUS:      \"$CPUS\" >> /results/run-stats
echo MEM:       \"$MEM\" >> /results/run-stats
if test -n "$RUN_ON_GCE"
then
    . /usr/local/lib/gce-funcs
    DMI_MEM=$(sudo dmidecode -t memory 2> /dev/null | \
		     grep "Maximum Capacity: " | \
		     sed -e 's/.*: //')
    if test $? -eq 0
    then
	echo "MEM: $DMI_MEM (Max capacity)" >> /results/run-stats
    fi
    PARAM_MEM=$(gce_attribute mem)
    if test -n "$PARAM_MEM"
    then
	echo "MEM: $PARAM_MEM (restricted by cmdline)" >> /results/run-stats
    fi
    GCE_ID=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/id" -H "Metadata-Flavor: Google" 2> /dev/null)
    echo GCE ID:    \"$GCE_ID\" >> /results/run-stats
    echo DATECODE: $DATECODE >> /results/run-stats
fi

cat /results/run-stats

for i in btrfs ext4 generic shared udf xfs config; do
    rm -rf /results/results-*/$i
done

cp /proc/slabinfo /results/slabinfo.before
cp /proc/meminfo /results/meminfo.before

free -m
for i in $FSTESTCFG
do
	export SCRATCH_DEV=$SM_SCR_DEV
	export SCRATCH_MNT=$SM_SCR_MNT
	export RESULT_BASE=/results/results-$i
	unset REQUIRE_FEATURE
	if test -e "/root/conf/$i"; then
		. "/root/conf/$i"
	else
		echo "Unknown configuration $i!"
		continue
	fi
	echo $i > /run/fstest-config
	if test -n "$EXT_MOUNT_OPTIONS" ; then
		EXT_MOUNT_OPTIONS="-o block_validity,$EXT_MOUNT_OPTIONS"
	else
		EXT_MOUNT_OPTIONS="-o block_validity"
	fi
	if test -n "$MNTOPTS" ; then
		EXT_MOUNT_OPTIONS="$EXT_MOUNT_OPTIONS,$MNTOPTS"
	fi
	mkdir -p "$RESULT_BASE"
	echo FS: $FS > "$RESULT_BASE/config"
	echo TESTNAME: $TESTNAME >> "$RESULT_BASE/config"
	echo TEST_DEV: $TEST_DEV >> "$RESULT_BASE/config"
	echo TEST_DIR: $TEST_DIR >> "$RESULT_BASE/config"
	echo SCRATCH_DEV: $SCRATCH_DEV >> "$RESULT_BASE/config"
	echo SCRATCH_MNT: $SCRATCH_MNT >> "$RESULT_BASE/config"
	echo MKFS_OPTIONS: $MKFS_OPTIONS >> "$RESULT_BASE/config"
	echo EXT_MOUNT_OPTIONS: $EXT_MOUNT_OPTIONS >> "$RESULT_BASE/config"
	if test "$TEST_DEV" != "$PRI_TST_DEV" ; then
		if test "$FS" = "ext4" ; then
		    mke2fs -F -q -t ext4 $MKFS_OPTIONS "$TEST_DEV"
		elif test "$FS" = "xfs" ; then
		    mkfs.xfs -f $MKFS_OPTIONS "$TEST_DEV"
		else
		    /sbin/mkfs.$FS "$TEST_DEV"
		fi
	fi
	echo 3 > /proc/sys/vm/drop_caches
	cp /proc/slabinfo "$RESULT_BASE/slabinfo.before"
	cp /proc/meminfo "$RESULT_BASE/meminfo.before"
	echo -n "BEGIN TEST $i: $TESTNAME " ; date
	logger "BEGIN TEST $i: $TESTNAME "
	if test -n "$REQUIRE_FEATURE" -a \
		! -f "/sys/fs/$FS/features/$REQUIRE_FEATURE" ; then
	    echo "END TEST: Kernel does not support $REQUIRE_FEATURE"
	    continue
	fi
	echo Device: $TEST_DEV
	echo mk2fs options: $MKFS_OPTIONS
	echo mount options: $EXT_MOUNT_OPTIONS
	export FSTYP=$FS
	AEX=""
	if test -n "$DO_AEX" ; then
	    sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
		< "/root/conf/all.exclude" > "/results/results-$i/exclude"
	    if test -f "/root/conf/$i.exclude"; then
		sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
		    < "/root/conf/$i.exclude" >> "/results/results-$i/exclude"
	    fi
	    if test $(stat -c %s "/results/results-$i/exclude") -gt 0 ; then
		AEX="-E /results/results-$i/exclude"
	    fi
        fi
	if test -f /tmp/exclude-tests ; then
	    AEX="$AEX -E /tmp/exclude-tests"
	fi
	gce_run_hooks fs-config-begin $i
	for j in $(seq 1 $RPT_COUNT) ; do
	    gce_run_hooks pre-xfstests $i $j
	    bash ./check -T $AEX $FSTESTSET
	    gce_run_hooks post-xfstests $i $j
	    umount $TEST_DEV >& /dev/null
	    if test "$FS" = "ext4" ; then
		/sbin/e2fsck -fy $TEST_DEV >& $RESULT_BASE/fsck.out
		if test $? -gt 0 ; then
		   cat $RESULT_BASE/fsck.out
		fi
	    elif test "$FS" = "xfs" ; then
		if ! xfs_repair -n "$TEST_DEV" >& /dev/null ; then
		    xfs_repair "$TEST_DEV"
		fi
	    else
		/sbin/fsck.$FS "$TEST_DEV"
	    fi
	done
	if test -n "$RUN_ON_GCE"
	then
	    gsutil cp "gs://$GS_BUCKET/check-time.tar.gz" /tmp >& /dev/null
	    if test -f /tmp/check-time.tar.gz
	    then
		tar -C /tmp -xzf /tmp/check-time.tar.gz
	    fi
	    if ! test -f "/tmp/check.time.$i"
	    then
		touch "/results/results-$i/check.time"
	    fi
	    cat "/results/results-$i/check.time" "/tmp/check.time.$i" \
		| awk '
	{ t[$1] = $2 }
END	{ if (NR > 0) {
	    for (i in t) print i " " t[i]
	  }
	}' \
		| sort -n > "/tmp/check.time.$i.new"
	    mv "/tmp/check.time.$i.new" "/tmp/check.time.$i"
	    (cd /tmp ; tar -cf - check.time.* | gzip -9 \
						     > /tmp/check-time.tar.gz)
	    gsutil cp /tmp/check-time.tar.gz "gs://$GS_BUCKET" >& /dev/null
	fi
	echo 3 > /proc/sys/vm/drop_caches
	cp /proc/slabinfo "$RESULT_BASE/slabinfo.after"
	cp /proc/meminfo "$RESULT_BASE/meminfo.after"
	free -m
	gce_run_hooks fs-config-end $i
	echo -n "END TEST: $TESTNAME " ; date
	logger "END TEST $i: $TESTNAME "
done

cp /proc/slabinfo /results/slabinfo.after
cp /proc/meminfo /results/meminfo.after
