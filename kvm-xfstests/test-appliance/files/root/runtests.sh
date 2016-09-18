#!/bin/bash

API_MAJOR=1
API_MINOR=3
. /root/test-config

function gce_run_hooks()
{
    if test -n "$RUN_ON_GCE"
    then
	run_hooks "$@"
    fi
}

function get_fs_config()
{
    local fs="$1"

    if test "$fs" == "$FS_CONFIGURED" ; then
	return
    fi
    FS_DIR="/root/fs/$fs"
    if test ! -d $FS_DIR ; then
	echo "File system $fs not supported"
	return 1
    fi
    . "$FS_DIR/config"
    FS_CONFIGURED="$fs"
    return 0
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
    no_punch)
	ALL_FSSTRESS_AVOID="$ALL_FSSTRESS_AVOID -f punch=0"
	ALL_FSX_AVOID="$ALL_FSX_AVOID -H"
	ALL_XFS_IO_AVOID="$ALL_XFS_IO_AVOID fpunch"
	FSTESTSET="$FSTESTSET -x punch"
	;;
    no_collapse)
	ALL_FSSTRESS_AVOID="$ALL_FSSTRESS_AVOID -f collapse=0"
	ALL_FSX_AVOID="$ALL_FSX_AVOID -C"
	ALL_XFS_IO_AVOID="$ALL_XFS_IO_AVOID fcollapse"
	FSTESTSET="$FSTESTSET -x collapse"
	;;
    no_insert)
	ALL_FSSTRESS_AVOID="$ALL_FSSTRESS_AVOID -f insert=0"
	ALL_FSX_AVOID="$ALL_FSX_AVOID -I"
	ALL_XFS_IO_AVOID="$ALL_XFS_IO_AVOID finsert"
	FSTESTSET="$FSTESTSET -x insert"
	;;
    no_zero)
	ALL_FSSTRESS_AVOID="$ALL_FSSTRESS_AVOID -f zero=0"
	ALL_FSX_AVOID="$ALL_FSX_AVOID -z"
	ALL_XFS_IO_AVOID="$ALL_XFS_IO_AVOID zero"
	FSTESTSET="$FSTESTSET -x zero"
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
if ! get_fs_config $FSTESTTYP ; then
    echo "Unsupported primary file system type $FSTESTTYP"
    exit 1
fi

if test "$(blkid -s TYPE -o value ""$PRI_TST_DEV"")" != "$FSTESTTYP"; then
    format_filesystem "$PRI_TST_DEV" "$DEFAULT_MKFS_OPTIONS"
fi
check_filesystem "$PRI_TST_DEV" >& "/tmp/fsck.$$"
FSCKCODE=$?
if test $FSCKCODE -gt 1
then
    cat /tmp/fsck.$$
fi

if test $FSCKCODE -ge 8
then
    format_filesystem "$PRI_TST_DEV" "$DEFAULT_MKFS_OPTIONS"
fi
if test ! -f /.dockerenv ; then
    dmesg -n 5
fi
cd /root/xfstests

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
if test -n "$RUN_ON_GCE"
then
    . /usr/local/lib/gce-funcs
    image=$(gcloud compute disks describe --format='value(sourceImage)' \
		${instance} | \
		sed -e 's;https://www.googleapis.com/compute/v1/projects/;;' \
		    -e 's;global/images/;;')
    echo "FSTESTIMG: $image" >> /results/run-stats
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

for i in $(find /results -name results-\* -type d)
do
    find $i/* -type d -print | xargs rm -rf 2> /dev/null
    find $i -type f ! -name check.time -print | xargs rm -f 2> /dev/null
done

cp /proc/slabinfo /results/slabinfo.before
cp /proc/meminfo /results/meminfo.before

free -m
while test -n "$FSTESTCFG"
do
	i="${FSTESTCFG%% *}"
	case "$FSTESTCFG" in
	    *\ *) FSTESTCFG="${FSTESTCFG#* }" ;;
	    *)    FSTESTCFG=""
	esac
	case "$i" in
	    */*)
		FS="${i%%/*}"
		i="${i#*/}"
		;;
	    *)
		if test -d "/root/fs/$i"
		then
		    FS="$i"
		    i=default
		else
		    FS="$FSTESTTYP"
		fi
		;;
	esac
	if test ! -d "/root/fs/$FS" ; then
	    echo "Unknown file system type $FS"
	    continue
	fi
	# Reset variables from the previous (potentially aborted) config
	unset SIZE REQUIRE_FEATURE
	unset FSX_AVOID FSSTRESS_AVOID XFS_IO_AVOID TEST_SET_EXCLUDE
	unset TEST_DEV TEST_DIR SCRATCH_DEV SCRATCH_MNT
	reset_vars
	get_fs_config "$FS"
	i=$(test_name_alias $i)
	if test -f "/root/fs/$FS/cfg/$i.list"; then
	    FSTESTCFG="$(cat /root/fs/$FS/cfg/$i.list | sed -e '/#/d' \
			-e '/^$/d' -e s:^:$FS/:) $FSTESTCFG"
	    FSTESTCFG="$(echo $FSTESTCFG)"
	    continue
	fi
	export SCRATCH_DEV=$SM_SCR_DEV
	export SCRATCH_MNT=$SM_SCR_MNT
	if test -f "/root/fs/$FS/cfg/$i"; then
		. "/root/fs/$FS/cfg/$i"
	else
		echo "Unknown configuration $FS/$i!"
		continue
	fi
	if test -z "$TEST_DEV" ; then
	    if test -z "$SIZE" ; then
		echo "No TEST_DEV and no SIZE"
		continue
	    fi
	    if test "$SIZE" = "large" ; then
		export TEST_DEV=$LG_TST_DEV
		export TEST_DIR=$LG_TST_MNT
	    else
		if test "$FSTESTTYP" = "$FS" -a \
		   "$DEFAULT_MKFS_OPTS" = "$(get_mkfs_opts)"
		then
		    export TEST_DEV=$PRI_TST_DEV
		    export TEST_DIR=$PRI_TST_MNT
		else
		    export TEST_DEV=$SM_TST_DEV
		    export TEST_DIR=$SM_TST_MNT
		fi
	    fi
	fi
	if test -z "$SCRATCH_DEV" ; then
	    if test "$SIZE" = "large" ; then
		export SCRATCH_DEV=$LG_SCR_DEV
		export SCRATCH_MNT=$LG_SCR_MNT
	    else
		export SCRATCH_DEV=$SM_SCR_DEV
		export SCRATCH_MNT=$SM_SCR_MNT
	    fi
	fi
	case "$TEST_DEV" in
	    */ovl) ;;
	    *:/*) ;;
	    *)
		if ! test -b $TEST_DEV ; then
		    echo "Test device $TEST_DEV does not exist, skipping $i config"
		    continue
		fi
		if ! test -b $SCRATCH_DEV ; then
		    echo "Scratch device $SCRATCH_DEV does not exist, skipping $i config"
		    continue
		fi
		;;
	esac
	if test -n "$ALL_FSX_AVOID"
	then
	    FSX_AVOID="$ALL_FSX_AVOID $FSX_AVOID"
	    FSX_AVOID="${FSX_AVOID/# /}"
	fi
	if test -n "$ALL_FSSTRESS_AVOID"
	then
	    FSSTRESS_AVOID="$ALL_FSSTRESS_AVOID $FSTRESS_AVOID"
	    FSSTRESS_AVOID="${FSSTRESS_AVOID/# /}"
	fi
	if test -n "$ALL_XFS_IO_AVOID"
	then
	    XFS_IO_AVOID="$ALL_XFS_IO_AVOID $XFS_IO_AVOID"
	    XFS_IO_AVOID="${XFS_IO_AVOID/# /}"
	fi
	echo $i > /run/fstest-config
	setup_mount_opts
	export RESULT_BASE="/results/$FS/results-$i"
	if test ! -d "$RESULT_BASE" -a -d "/results/results-$i" ; then
	    mkdir -p "/results/$FS"
	    mv "/results/results-$i" "$RESULT_BASE"
	fi
	mkdir -p "$RESULT_BASE"
	echo FS: $FS > "$RESULT_BASE/config"
	echo TESTNAME: $TESTNAME >> "$RESULT_BASE/config"
	echo TEST_DEV: $TEST_DEV >> "$RESULT_BASE/config"
	echo TEST_DIR: $TEST_DIR >> "$RESULT_BASE/config"
	echo SCRATCH_DEV: $SCRATCH_DEV >> "$RESULT_BASE/config"
	echo SCRATCH_MNT: $SCRATCH_MNT >> "$RESULT_BASE/config"
	show_mkfs_opts >> "$RESULT_BASE/config"
	show_mount_opts >> "$RESULT_BASE/config"
	if test "$TEST_DEV" != "$PRI_TST_DEV" ; then
	    format_filesystem "$TEST_DEV" "$(get_mkfs_opts)"
	fi
	if test ! -f /.dockerenv ; then
	    echo 3 > /proc/sys/vm/drop_caches
	fi
	cp /proc/slabinfo "$RESULT_BASE/slabinfo.before"
	cp /proc/meminfo "$RESULT_BASE/meminfo.before"
	echo -n "BEGIN TEST $i: $TESTNAME " ; date
	logger "BEGIN TEST $i: $TESTNAME "
	if test -n "$REQUIRE_FEATURE" -a \
		! -f "/sys/fs/$FS/features/$REQUIRE_FEATURE" ; then
	    echo "END TEST: Kernel does not support $REQUIRE_FEATURE"
	    continue
	fi
	echo DEVICE: $TEST_DEV
	show_mkfs_opts
	show_mount_opts
	if test -n "$FSX_AVOID"
	then
	    echo FSX_AVOID: $FSX_AVOID
	    echo FSX_AVOID: $FSX_AVOID >> "$RESULT_BASE/config"
	    export FSX_AVOID
	fi
	if test -n "$FSSTRESS_AVOID"
	then
	    echo FSSTRESS_AVOID: $FSSTRESS_AVOID
	    echo FSSTRESS_AVOID: $FSSTRESS_AVOID >> "$RESULT_BASE/config"
	    export FSSTRESS_AVOID
	fi
	if test -n "$XFS_IO_AVOID"
	then
	    echo XFS_IO_AVOID: $XFS_IO_AVOID
	    echo XFS_IO_AVOID: $XFS_IO_AVOID >> "$RESULT_BASE/config"
	    export XFS_IO_AVOID
	fi
	if test -n "$TEST_SET_EXCLUDE"
	then
	    echo TEST_SET_EXCLUDE: $TEST_SET_EXCLUDE
	    echo TEST_SET_EXCLUDE: $XFS_IO_AVOID >> "$RESULT_BASE/config"
	fi
	export FSTYP=$FS
	AEX=""
	if test -n "$DO_AEX" ; then
	    if test -f "/root/fs/$FS/exclude" ; then
		sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
		    < "/root/fs/$FS/exclude" > "$RESULT_BASE/exclude"
	    else
		cp /dev/null "$RESULT_BASE/exclude"
	    fi
	    if test -f "/root/fs/$FS/cfg/$i.exclude"; then
		sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
		    < "/root/fs/$FS/cfg/$i.exclude" >> "$RESULT_BASE/exclude"
	    fi
	    if test $(stat -c %s "$RESULT_BASE/exclude") -gt 0 ; then
		AEX="-E $RESULT_BASE/exclude"
	    fi
        fi
	if test -f /tmp/exclude-tests ; then
	    AEX="$AEX -E /tmp/exclude-tests"
	fi
	gce_run_hooks fs-config-begin $i
	for j in $(seq 1 $RPT_COUNT) ; do
	    gce_run_hooks pre-xfstests $i $j
	    bash ./check -T $AEX $FSTESTSET $TEST_SET_EXCLUDE
	    gce_run_hooks post-xfstests $i $j
	    umount "$TEST_DEV" >& /dev/null
	    check_filesystem "$TEST_DEV" >& $RESULT_BASE/fsck.out
	    if test $? -gt 0 ; then
		cat $RESULT_BASE/fsck.out
	    fi
	done
	if test -n "$RUN_ON_GCE"
	then
	    gsutil cp "gs://$GS_BUCKET/check-time.tar.gz" /tmp >& /dev/null
	    if test -f /tmp/check-time.tar.gz
	    then
		tar -C /tmp -xzf /tmp/check-time.tar.gz
	    fi
	    check_time="/tmp/check.time.$FS.$i"
	    if test ! -f "$check_time" -a -f "/tmp/check.time.$i"; then
		mv "/tmp/check.time.$i" "$check_time"
	    fi
	    touch "$RESULT_BASE/check.time" "$check_time"
	    cat "$RESULT_BASE/check.time" "$check_time" \
		| awk '
	{ t[$1] = $2 }
END	{ if (NR > 0) {
	    for (i in t) print i " " t[i]
	  }
	}' \
		| sort -n > "${check_time}.new"
	    mv "${check_time}.new" "$check_time"
	    (cd /tmp ; tar -cf - check.time.* | gzip -9 \
						     > /tmp/check-time.tar.gz)
	    gsutil cp /tmp/check-time.tar.gz "gs://$GS_BUCKET" >& /dev/null
	fi
	if test ! -f /.dockerenv ; then
	    echo 3 > /proc/sys/vm/drop_caches
	fi
	cp /proc/slabinfo "$RESULT_BASE/slabinfo.after"
	cp /proc/meminfo "$RESULT_BASE/meminfo.after"
	free -m
	gce_run_hooks fs-config-end $i
	umount "$TEST_DIR" >& /dev/null
	umount "$SCRATCH_MNT" >& /dev/null
	echo -n "END TEST: $TESTNAME " ; date
	logger "END TEST $i: $TESTNAME "
done

cp /proc/slabinfo /results/slabinfo.after
cp /proc/meminfo /results/meminfo.after
