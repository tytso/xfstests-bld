#!/bin/bash

API_MAJOR=1
API_MINOR=1
. /root/test-config

if test -z "$FSTESTAPI" ; then
    echo "Missing TEST API!"
    umount /results
    poweroff -f
fi

set $FSTESTAPI

if test "$1" -ne "$API_MAJOR" ; then
    echo " "
    echo "API version of kvm-xfstests is $1.$2"
    echo "Major version number must be $API_MAJOR"
    echo " "
    umount /results
    poweroff -f
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
	echo "Enabling auto exclude"
	DO_AEX=t
	;;
    count) shift
	RPT_COUNT=$1
	echo "Repeat each test $RPT_COUNT times"
	;;
    *)
	echo " "
	echo "Unrecognized option $i"
	echo " "
  esac
  shift
done

umount $PRI_TST_DEV >& /dev/null
umount $SM_TST_DEV >& /dev/null
/sbin/e2fsck -fy $PRI_TST_DEV
if test $? -ge 8 ; then
	mke2fs -F -q -t ext4 $PRI_TST_DEV
fi
dmesg -n 5
cd /root/xfstests

if test "$FSTESTCFG" = all
then
	FSTESTCFG="4k 1k ext3 nojournal ext3conv metacsum dioread_nolock data_journal inline bigalloc bigalloc_1k"
fi

if test -n "$FSTESTEXC" ; then
	echo $FSTESTEXC | tr , \\n > /tmp/exclude-tests
else
	rm -f /tmp/exclude-tests
fi

sed -e 's/^/FSTESTVER: /g' /root/xfstests/git-versions > /results/run-stats
echo -e "FSTESTVER: kernel\t$(uname -r -v -m)" >> /results/run-stats
echo FSTESTCFG: \"$FSTESTCFG\" >> /results/run-stats
echo FSTESTSET: \"$FSTESTSET\" >> /results/run-stats
echo FSTESTEXC: \"$FSTESTEXC\" >> /results/run-stats
echo FSTESTOPT: \"$FSTESTOPT\" >> /results/run-stats
echo MNTOPTS: \"$MNTOPTS\" >> /results/run-stats

cat /results/run-stats

for i in btrfs ext4 generic shared udf xfs config; do
    rm -rf /results/results-*/$i
done

for i in $FSTESTCFG
do
	export SCRATCH_DEV=$SM_SCR_DEV
	export SCRATCH_MNT=$SM_SCR_MNT
	export RESULT_BASE=/results/results-$i
	if test -e "/root/conf/$i"; then
		. /root/conf/$i
	else
		echo "Unknown configuration $i!"
		continue
	fi
	mkdir -p $RESULT_BASE
	echo FS: $FS > $RESULT_BASE/config
	echo TESTNAME: $TESTNAME >> $RESULT_BASE/config
	echo TEST_DEV: $TEST_DEV >> $RESULT_BASE/config
	echo TEST_DIR: $TEST_DIR >> $RESULT_BASE/config
	echo SCRATCH_DEV: $SCRATCH_DEV >> $RESULT_BASE/config
	echo SCRATCH_MNT: $SCRATCH_MNT >> $RESULT_BASE/config
	echo MKFS_OPTIONS: $MKFS_OPTIONS >> $RESULT_BASE/config
	echo EXT_MOUNT_OPTIONS: $EXT_MOUNT_OPTIONS >> $RESULT_BASE/config
	if test -n "$MNTOPTS" ; then
		EXT_MOUNT_OPTIONS="$EXT_MOUNT_OPTIONS,$MNTOPTS"
	fi
	if test "$TEST_DEV" != "$PRI_TST_DEV" ; then
		if test "$FS" = "ext4" ; then
		    mke2fs -F -q -t ext4 $MKFS_OPTIONS $TEST_DEV
		elif test "$FS" = "xfs" ; then
		    mkfs.xfs -f $MKFS_OPTIONS $TEST_DEV
		else
		    /sbin/mkfs.$FS $TEST_DEV
		fi
	fi
	if test "$FS" = "ext4" ; then
	    SLAB_GREP="ext4\|jbd2"
	else
	    SLAB_GREP=$FS
	fi
	echo 3 > /proc/sys/vm/drop_caches
	if test "$SLAB_GREP" != "$OLD_SLAB_GREP" ; then
	    free -m
	    grep $SLAB_GREP /proc/slabinfo
	    OLD_SLAB_GREP="$SLAB_GREP"
	fi
	echo -n "BEGIN TEST: $TESTNAME " ; date
	echo Device: $TEST_DEV
	echo mk2fs options: $MKFS_OPTIONS
	echo mount options: $EXT_MOUNT_OPTIONS
	export FSTYP=$FS
	AEX=""
	if test -n "$DO_AEX" -a -f "/root/conf/$i.exclude"; then
	    AEX="-E /root/conf/$i.exclude"
        fi
	if test -f /tmp/exclude-tests ; then
	    AEX="$AEX -E /tmp/exclude-tests"
	fi
	for j in $(seq 1 $RPT_COUNT) ; do
	   bash ./check -T $AEX $FSTESTSET
	   umount $TEST_DEV >& /dev/null
	   if test "$FS" = "ext4" ; then
		/sbin/e2fsck -fy $TEST_DEV >& $RESULT_BASE/fsck.out
		if test $? -gt 0 ; then
		   cat $RESULT_BASE/fsck.out
		fi
	   elif test "$FS" = "xfs" ; then
		if ! xfs_repair -n $TEST_DEV >& /dev/null ; then
		   xfs_repair $TEST_DEV
		fi
	   else
		/sbin/fsck.$FS $TEST_DEV
	   fi
	done
	echo 3 > /proc/sys/vm/drop_caches
	free -m
	grep $SLAB_GREP /proc/slabinfo
	echo -n "END TEST: $TESTNAME " ; date
done
