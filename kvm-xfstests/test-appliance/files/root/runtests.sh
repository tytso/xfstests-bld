#!/bin/bash

API_MAJOR=1
API_MINOR=0
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

umount $VDB >& /dev/null
umount $VDD >& /dev/null
/sbin/e2fsck -fy $VDB
if test $? -ge 8 ; then
	mke2fs -F -q -t ext4 $VDB
fi
dmesg -n 5
cd /root/xfstests

if test "$FSTESTCFG" = all
then
	FSTESTCFG="4k 1k ext3 nojournal ext3conv metacsum dioread_nolock data_journal bigalloc bigalloc_1k inline"
fi

SLAB_GREP="ext4\|jbd2\|xfs"

grep $SLAB_GREP /proc/slabinfo
free -m

for i in $FSTESTCFG
do
	export SCRATCH_DEV=$VDC
	export SCRATCH_MNT=/vdc
	export RESULT_BASE=/results/results-$i
	mkdir -p $RESULT_BASE
	if test -e "/root/conf/$i"; then
		. /root/conf/$i
	else
		echo "Unknown configuration $i!"
		continue
	fi
	if test -n "$MNTOPTS" ; then
		EXT_MOUNT_OPTIONS="$EXT_MOUNT_OPTIONS,$MNTOPTS"
	fi
	if test "$TEST_DEV" != "$VDB" ; then
		if test "$FS" = "ext4" ; then
		    mke2fs -F -q -t ext4 $MKFS_OPTIONS $TEST_DEV
		elif test "$FS" = "xfs" ; then
		    mkfs.xfs -f $MKFS_OPTIONS $TEST_DEV
		else
		    /sbin/mkfs.$FS $TEST_DEV
		fi
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
	free -m
	if test "$FS" = "ext4" ; then
	   SLAB_GREP="ext4\|jbd2"
	else
	   SLAB_GREP=$FS
	fi
	grep $SLAB_GREP /proc/slabinfo
	echo -n "END TEST: $TESTNAME " ; date
done
