#!/bin/bash
. /root/test-config
umount $VDB
umount $VDD
/sbin/e2fsck -fy $VDB
if test $? -ge 8 ; then
	mke2fs -t ext4 $VDB
fi
dmesg -n 5
cd /root/xfstests

if test "$FSTESTCFG" = all
then
	FSTESTCFG="4k ext3 nojournal 1k ext3conv metacsum dioread_nolock data_journal bigalloc bigalloc_1k"
fi

if test -n "$(echo $FSTESTSET | awk '/^AEX /{print "t"}')"
then
	echo "Enabling auto exclude"
	DO_AEX=t
	FSTESTSET=$(echo $FSTESTSET | sed -e 's/^AEX //')
fi

SLAB_GREP="ext4\|jbd2\|xfs"

grep $SLAB_GREP /proc/slabinfo
free -m
echo git versions:
cat git-versions

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
	if test "$TEST_DEV" != "$VDB" ; then
		if test "$FS" = "ext4" ; then
		    mke2fs -q -t ext4 $MKFS_OPTIONS $TEST_DEV
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
	if test -n "$DO_AEX" ; then
	    AEX="-X $i.exclude"
        fi
	bash ./check -T $AEX $FSTESTSET
	free -m
	if test "$FS" = "ext4" ; then
	   SLAB_GREP="ext4\|jbd2"
	else
	   SLAB_GREP=$FS
	fi
	grep $SLAB_GREP /proc/slabinfo
	echo -n "END TEST: $TESTNAME " ; date
	umount $TEST_DEV >& /dev/null
	if test "$FS" = "ext4" ; then
		/sbin/e2fsck -fy $TEST_DEV
	elif test "$FS" = "xfs" ; then
		xfs_repair -f $TEST_DEV
	else
		/sbin/fsck.$FS $TEST_DEV
	fi
done
