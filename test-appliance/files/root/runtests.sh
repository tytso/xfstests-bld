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

function copy_xunit_results()
{
    local RESULT="$RESULT_BASE/result.xml"
    local RESULTS="$RESULT_BASE/results.xml"

    if test -f "$RESULT"
    then
	if test -f "$RESULTS"
	then
	    merge_xunit "$RESULTS" "$RESULT"
	else
	    if ! update_properties_xunit --fsconfig "$FS/$TC" "$RESULTS" \
		 "$RESULT" "$RUNSTATS"
	    then
		mv "$RESULT" "$RESULT.broken"
	    fi
	fi
	rm "$RESULT"
    fi

    /root/xfstests/bin/syncfs $RESULT_BASE
}

# check to see if a device is assigned to be used
function is_dev_free() {
    local device="$1"

    for dev in "$TEST_DEV" \
	       "$SCRATCH_DEV" \
	       "$SCRATCH_LOGDEV" \
	       "$TEST_LOGDEV" \
	       "$LOGWRITES_DEV" \
	       "$SCRATCH_RTDEV" \
	       "$TEST_RTDEV"
    do
	if test "$dev" == "$1" ; then
	    return 1
	fi
    done
    return 0
}

gen_version_header ()
{
    local version patchlevel sublevel

    read version patchlevel sublevel <<< \
	 $(uname -r | sed -e 's/-.*$//' | tr . ' ')

    echo '#define KERNEL_VERSION(a,b,c) (((a) << 16) + ((b) << 8) + \
	((c) > 255 ? 255 : (c)))'
    echo \#define LINUX_VERSION_MAJOR $version
    echo \#define LINUX_VERSION_PATCHLEVEL $patchlevel
    echo \#define LINUX_VERSION_SUBLEVEL $sublevel
    if [ $sublevel -gt 255 ]; then
	sublevel=255
    fi
    echo \#define LINUX_VERSION_CODE \
	$(expr $version \* 65536 + $patchlevel \* 256 + $sublevel)
    test -n "$FS" && echo \#define FC $FS
    test -n "$TC" && echo \#define TC $TC
    test "$TC" = dax && echo \#define IS_DAX_CONFIG
}

function clear_pool_devs ()
{
    if test -n "$POOL0_DEV" ; then
	losetup -d "$POOL0_DEV"
	POOL0_DEV=
    fi
    if test -n "$POOL1_DEV" ; then
	losetup -d "$POOL1_DEV"
	POOL1_DEV=
    fi
    if test -n "$POOL2_DEV" ; then
	losetup -d "$POOL2_DEV"
	POOL2_DEV=
    fi
    if test -n "$POOL3_DEV" ; then
	losetup -d "$POOL3_DEV"
	POOL3_DEV=
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
FAIL_LOOP_COUNT=4

while [ "$1" != "" ]; do
  case $1 in
    aex)
	DO_AEX=t
	;;
    count) shift
	RPT_COUNT=$1
	FAIL_LOOP_COUNT=0
	;;
    fail_loop_count) shift
	FAIL_LOOP_COUNT=$1
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

umount "$PRI_TST_DEV" >& /dev/null
umount "$SM_TST_DEV" >& /dev/null
if ! get_fs_config $FSTESTTYP ; then
    echo "Unsupported primary file system type $FSTESTTYP"
    exit 1
fi

if test -b "$PRI_TST_DEV" ; then
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

if test -n "$RUN_ONCE" -a -f "$RUNSTATS"
then
    mv "$RUNSTATS" "$RUNSTATS.old"
    RESTARTED=yes
fi

cp /dev/null "$RUNSTATS"
echo CMDLINE: \"$(echo $ORIG_CMDLINE | base64 -d)\" >> "$RUNSTATS"
if test -n "$RUN_ON_GCE"
then
    cp /usr/local/lib/gce-local.config /root/xfstests/local.config
    . /usr/local/lib/gce-funcs
    image=$(gcloud compute disks describe --format='value(sourceImage)' \
		--zone "$ZONE" ${instance} | \
		sed -e 's;https://www.googleapis.com/compute/v1/projects/;;' \
		    -e 's;global/images/;;')
    echo "FSTESTIMG: $image" >> "$RUNSTATS"
    echo "FSTESTPRJ: $(get_metadata_value_with_retries project-id)" >> "$RUNSTATS"
fi
echo -e "KERNEL: kernel\t$(uname -r -v -m)" >> "$RUNSTATS"
sed -e 's/^/FSTESTVER: /g' /root/xfstests/git-versions >> "$RUNSTATS"
echo FSTESTCFG: \"$FSTESTCFG\" >> "$RUNSTATS"
echo FSTESTSET: \"$FSTESTSET\" >> "$RUNSTATS"
echo FSTESTEXC: \"$FSTESTEXC\" >> "$RUNSTATS"
echo FSTESTOPT: \"$FSTESTOPT\" >> "$RUNSTATS"
echo MNTOPTS:   \"$MNTOPTS\" >> "$RUNSTATS"
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
    echo GCE ID:    \"$GCE_ID\" >> "$RUNSTATS"
    MACHTYPE=$(basename $(get_metadata_value_with_retries machine-type))
    echo MACHINE TYPE: \"$MACHTYPE\" >> "$RUNSTATS"
    echo TESTRUNID: $TESTRUNID >> "$RUNSTATS"
fi

if test -z "$RUN_ON_GCE" -o -z "$RUN_ONCE"
then
    for i in $(find "$RESULTS" -name results-\* -type d)
    do
	if [ "$(ls -A $i)" ]; then
	    find $i/* -type d -print | xargs rm -rf 2> /dev/null
	    find $i -type f ! -name check.time -print | xargs rm -f 2> /dev/null
	fi
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

./check --help > /tmp/check-help
report_fmt=xunit
if grep -q xunit-quiet /tmp/check-help ; then
    report_fmt=xunit-quiet
fi
fail_test_loop=
if test $RPT_COUNT -eq 1 && test $FAIL_LOOP_COUNT -gt 0 && \
	grep -q -- "-L <n>" /tmp/check-help ; then
    fail_test_loop="-L $FAIL_LOOP_COUNT"
fi

[ -e /proc/slabinfo ] && cp /proc/slabinfo "$RESULTS/slabinfo.before"
cp /proc/meminfo "$RESULTS/meminfo.before"

if test -n "$FSTESTSTR" ; then
    systemctl start stress
fi

while test -n "$FSTESTCFG"
do
	clear_pool_devs
	if ! get_one_fs_config "/root/fs"; then
          continue
        fi
	if test -z "$RUN_ON_GCE" -a -n "$USE_FILESTORE" ; then
	    echo -n "BEGIN TEST $TC: $TESTNAME " ; date
	    logger "BEGIN TEST $TC: $TESTNAME "
	    echo "END TEST: $FS/$TC only supported on gce-xfstests"
	    logger "END TEST: $FS/$TC only supported on gce-xfstests"
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
			-b "$PRI_TST_DEV" -a \
			"$DEFAULT_MKFS_OPTIONS" = "$(get_mkfs_opts)"
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
		export LOGWRITES_DEV=$SM_SCR_DEV
	    else
		export SCRATCH_DEV=$SM_SCR_DEV
		export SCRATCH_MNT=$SM_SCR_MNT
		export LOGWRITES_DEV=$LG_SCR_DEV
	    fi
	fi

	if test "$SCRATCH_LOGDEV" = "/dev/XXX" ; then
		if [ -z "$TINY_SCR_DEV" ]; then
			echo "Error: No TINY_SCR_DEV set, skipping config $FS/$TC"
			continue
		fi

		export SCRATCH_LOGDEV="$TINY_SCR_DEV"
		export USE_EXTERNAL=yes
	fi

	if test "$TEST_LOGDEV" = "/dev/XXX" ; then
		if [ -z "$TINY_TST_DEV" ]; then
			echo "Error: No TINY_SCR_DEV set, skipping config $FS/$TC"
			continue
		fi

		export TEST_LOGDEV="$TINY_TST_DEV"
		export USE_EXTERNAL=yes
	fi

	if test "$SCRATCH_RTDEV" = "/dev/XXX" ; then
	    export USE_EXTERNAL=yes
	    if is_dev_free "$SM_SCR_DEV" ; then
		export SCRATCH_RTDEV="$SM_SCR_DEV"
	    elif is_dev_free "$LG_SCR_DEV" ; then
		export SCRATCH_RTDEV="$LG_SCR_DEV"
	    elif is_dev_free "$SM_TST_DEV" ; then
		export SCRATCH_RTDEV="$SM_TST_DEV"
	    elif is_dev_free "$LG_TST_DEV"; then
		export SCRATCH_RTDEV="$LG_TST_DEV"
	    elif is_dev_free "$PRI_TST_DEV" ; then
		export SCRATCH_RTDEV="$PRI_TST_DEV"
	    else
		echo "WARNING: no available disk for SCRATCH_RTDEV"
	    fi
	fi

	if test "$TEST_RTDEV" = "/dev/XXX" ; then
	    export USE_EXTERNAL=yes
	    if is_dev_free "$SM_SCR_DEV" ; then
		export TEST_RTDEV="$SM_SCR_DEV"
	    elif is_dev_free "$LG_SCR_DEV" ; then
		export TEST_RTDEV="$LG_SCR_DEV"
	    elif is_dev_free "$SM_TST_DEV" ; then
		export TEST_RTDEV="$SM_TST_DEV"
	    elif is_dev_free "$LG_TST_DEV"; then
		export TEST_RTDEV="$LG_TST_DEV"
	    elif is_dev_free "$PRI_TST_DEV" ; then
		export TEST_RTDEV="$PRI_TST_DEV"
	    else
		echo "WARNING: no available disk for SCRATCH_RTDEV"
	    fi
	fi

	# This is required in case of BTRFS uses SCRATCH_DEV_POOL
	if [[ -n $SCRATCH_DEV_POOL ]]; then
	    lopt="--sizelimit 5GiB -L -f --show"
	    POOL0_DEV=$(losetup $lopt -o  0GiB "$LG_SCR_DEV")
	    POOL1_DEV=$(losetup $lopt -o  5GiB "$LG_SCR_DEV")
	    POOL2_DEV=$(losetup $lopt -o 10GiB "$LG_SCR_DEV")
	    POOL3_DEV=$(losetup $lopt -o 15GiB "$LG_SCR_DEV")
	    SCRATCH_DEV_POOL="$SCRATCH_DEV $POOL0_DEV $POOL1_DEV $POOL2_DEV $POOL3_DEV"
	    unset SCRATCH_DEV
	fi

	case "$TEST_DEV" in
	    */ovl|9p*) ;;
	    *:/*) ;;
	    *)
		if ! [ -b $TEST_DEV -o -c $TEST_DEV ]; then
		    echo "Test device $TEST_DEV does not exist, skipping $TC config"
		    continue
		fi
		if ! [ -b $SCRATCH_DEV -o -c $SCRATCH_DEV ]; then
		    echo "Scratch device $SCRATCH_DEV does not exist, skipping $TC config"
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
	echo $FS/$TC > /run/fstest-config
	if test -n "$RUN_ONCE" && \
		grep -q "^$FS/$TC\$" "$RESULTS/fstest-completed"
	then
	    echo "$FS/$TC: already run"
	    /usr/local/lib/gce-logger already run
	    continue
	fi
	setup_mount_opts
	export RESULT_BASE="$RESULTS/$FS/results-$TC"
	if test ! -d "$RESULT_BASE" -a -d "$RESULTS/results-$TC" ; then
	    mkdir -p "$RESULTS/$FS"
	    mv "$RESULTS/results-$TC" "$RESULT_BASE"
	fi
	mkdir -p "$RESULT_BASE"
	copy_xunit_results
	echo FS: $FS > "$RESULT_BASE/config"
	echo TESTNAME: $TESTNAME >> "$RESULT_BASE/config"
	echo TEST_DEV: $TEST_DEV >> "$RESULT_BASE/config"
	echo TEST_DIR: $TEST_DIR >> "$RESULT_BASE/config"
	echo SCRATCH_DEV: $SCRATCH_DEV >> "$RESULT_BASE/config"
	echo SCRATCH_DEV_POOL: $SCRATCH_DEV_POOL >> "$RESULT_BASE/config"
	echo SCRATCH_MNT: $SCRATCH_MNT >> "$RESULT_BASE/config"
	echo SCRATCH_LOGDEV: $SCRATCH_LOGDEV >> "$RESULT_BASE/config"
	echo TEST_LOGDEV: $TEST_LOGDEV >> "$RESULT_BASE/config"
	echo SCRATCH_RTDEV: $SCRATCH_RTDEV >> "$RESULT_BASE/config"
	echo TEST_RTDEV: $TEST_RTDEV >> "$RESULT_BASE/config"
	show_mkfs_opts >> "$RESULT_BASE/config"
	show_mount_opts >> "$RESULT_BASE/config"
	if test -n "$SCRATCH_DEV_POOL" ; then
	    losetup --list -a > "$RESULT_BASE/loop-devices"
	fi
	if test "$TEST_DEV" != "$PRI_TST_DEV" ; then
	    format_filesystem "$TEST_DEV" "$(get_mkfs_opts)"
	    ret="$?"
	    if test "$ret" -gt 0 ; then
		echo "Failed to format file system: exit status $ret"
		continue
	    fi
	fi
	if test ! -f /.dockerenv ; then
	    echo 3 > /proc/sys/vm/drop_caches
	fi
	[ -e /proc/slabinfo ] && cp /proc/slabinfo "$RESULT_BASE/slabinfo.before"
	cp /proc/meminfo "$RESULT_BASE/meminfo.before"
	if test -n "$REQUIRE_FEATURE" -a \
		! -f "/sys/fs/$FS/features/$REQUIRE_FEATURE" ; then
	    echo -n "BEGIN TEST $TC: $TESTNAME " ; date
	    logger "BEGIN TEST $TC: $TESTNAME "
	    echo "END TEST: Kernel does not support $REQUIRE_FEATURE"
	    logger "END TEST: Kernel does not support $REQUIRE_FEATURE"
	    continue
	fi
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
	    files=()
	    for i in "/root/fs/global_exclude" \
			"/root/fs/$FS/exclude" \
			"/root/fs/$FS/cfg/$TC.exclude" \
			"/root/fs/exclude.$XFSTESTS_FLAVOR" ; do
		test -f "$i" && files+=("$i")
	    done
	    if [ ${#files[@]} -ge 0 ]; then
		sed -e 's;//.*;;' -e 's/[ \t]*$//' -e '/^$/d' \
		    ${files[@]} > /tmp/exclude.cpp
	    else
		cp /dev/null /tmp/exclude.cpp
	    fi
	    if test -s "/tmp/exclude.cpp" ; then
		gen_version_header > /tmp/header.cpp
		cat /tmp/header.cpp /tmp/exclude.cpp | \
		    cpp -I /root/fs/$FS/cfg | \
		    sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
			> /tmp/exclude
	    fi
	    if test -s "/tmp/exclude" ; then
		EXSET=$(sort -u "/tmp/exclude")
		bash ./check -n $EXSET | \
		    sed -e '1,/^$/d' -e '/^$/d' | \
		    sort -u > "$RESULT_BASE/exclude"
		AEX="-E $RESULT_BASE/exclude"
	    fi
        fi
	rm -f "$RESULT_BASE/exclude-opt"
	if test -f "/root/fs/$FS/exclude-opt" ; then
	    AEX="$AEX $(cat /root/fs/$FS/exclude-opt)"
	    cat /root/fs/$FS/exclude-opt >> "$RESULT_BASE/exclude-opt"
	fi
	if test -f "/root/fs/$FS/cfg/exclude-opt" ; then
	    AEX="$AEX $(cat /root/fs/$FS/cfg/exclude-opt)"
	    cat /root/fs/cfg/$FS/exclude-opt >> "$RESULT_BASE/exclude-opt"
	fi
	if test -f /tmp/exclude-tests ; then
	    AEX="$AEX -E /tmp/exclude-tests"
	fi
	if test ! -f "$RESULT_BASE/tests-to-run" ; then
	    bash ./check -n $FSTESTSET >& /tmp/tests-to-run.debug
	    ret="$?"
	    echo "Exit status $ret" >> /tmp/tests-to-run.debug
	    if test "$ret" -gt 0 ; then
		echo "Failed to run ./check -n $FSTESTSET"
		cat /tmp/tests-to-run.debug
		continue
	    fi
	    bash ./check -n $FSTESTSET 2> /dev/null | \
		sed -e '1,/^$/d' -e '/^$/d' | \
		sort > "$RESULT_BASE/tests-to-run"
	    nr_tests=$(wc -l < "$RESULT_BASE/tests-to-run")
	    if test "$nr_tests" -ne 1
	    then
		nr_tests="$nr_tests tests"
	    else
		nr_tests="$nr_tests test"
	    fi
	    echo -n "BEGIN TEST $TC ($nr_tests): $TESTNAME " ; date
	    logger "BEGIN TEST $TC: $TESTNAME "
	    echo DEVICE: $TEST_DEV
	    show_mkfs_opts
	    show_mount_opts
	fi
	gce_run_hooks fs-config-begin $TC
	RPT_START=1
	if test -f "$RESULT_BASE/rpt_status"; then
	    RPT_START=$(cat "$RESULT_BASE/rpt_status" | sed 's:/.*::g')
	fi
	for j in $(seq $RPT_START $RPT_COUNT) ; do
	    echo "$j/$RPT_COUNT" > "$RESULT_BASE/rpt_status"
	    /root/xfstests/bin/syncfs "$RESULT_BASE"
	    gce_run_hooks pre-xfstests $TC $j
	    if test -n "$RUN_ONCE" ; then
		if test -f "$RESULT_BASE/completed"
		then
		    last_test="$(tail -n 1 "$RESULT_BASE/completed")"

		    if test -f "$RESULT_BASE/results.xml"; then
			add_error_xunit "$RESULT_BASE/results.xml" "$last_test" "xfstests.global"
		    else
			# if first test crashes, make sure results.xml gets
			# setup correctly via copy_xunit_results
			add_error_xunit "$RESULT_BASE/result.xml" "$last_test" "xfstests.global"
			copy_xunit_results
		    fi
		    /root/xfstests/bin/syncfs $RESULT_BASE

		    # this was part of the in-progress preemption work,
		    # removing for now as it conflicts with the crash recovery stuff
		    # head -n -2 "$RESULT_BASE/completed" > /tmp/completed
		    # mv /tmp/completed "$RESULT_BASE/completed"
		else
		    touch "$RESULT_BASE/completed"
		fi
		sort "$RESULT_BASE/completed" > /tmp/completed
		comm -23 "$RESULT_BASE/tests-to-run" /tmp/completed \
		     > /tmp/tests-to-run
	    else
		cp "$RESULT_BASE/tests-to-run" /tmp/tests-to-run
	    fi
	    if test -s /tmp/tests-to-run
	    then
		echo ./check -R $report_fmt $fail_test_loop -T $EXTRA_OPT \
		     $AEX $TEST_SET_EXCLUDE $(cat /tmp/tests-to-run) \
		     >> "$RESULT_BASE/check-cmd"
		bash ./check -R $report_fmt $fail_test_loop -T $EXTRA_OPT \
		     $AEX $TEST_SET_EXCLUDE $(cat /tmp/tests-to-run)
		copy_xunit_results
	    else
		echo "No tests to run"
	    fi
	    gce_run_hooks post-xfstests $TC $j
	    umount "$TEST_DEV" >& /dev/null
	    check_filesystem "$TEST_DEV" >& $RESULT_BASE/fsck.out
	    if test $? -gt 0 ; then
		cat $RESULT_BASE/fsck.out
	    fi
	    rm -f "$RESULT_BASE/completed"
	done
	rm -f "$RESULT_BASE/rpt_status"
	if test -n "$RUN_ON_GCE"
	then
	    gsutil cp "gs://$GS_BUCKET/check-time.tar.gz" /tmp >& /dev/null
	    if test -f /tmp/check-time.tar.gz
	    then
		tar -C /tmp -xzf /tmp/check-time.tar.gz
	    fi
	    check_time="/tmp/check.time.$FS.$TC"
	    if test ! -f "$check_time" -a -f "/tmp/check.time.$TC"; then
		mv "/tmp/check.time.$TC" "$check_time"
	    fi
	    touch "$RESULT_BASE/check.time" "$check_time"
	    cat "$check_time" "$RESULT_BASE/check.time" \
		| awk '
	{ t[$1] = $2 }
END	{ if (NR > 0) {
	    for (i in t) print i " " t[i]
	  }
	}' \
		| sort > "${check_time}.new"
	    mv "${check_time}.new" "$check_time"
	    (cd /tmp ; tar -cf - check.time.* | gzip -9 \
						     > /tmp/check-time.tar.gz)
	    gsutil cp /tmp/check-time.tar.gz "gs://$GS_BUCKET" >& /dev/null
	fi
	if test ! -f /.dockerenv ; then
	    echo 3 > /proc/sys/vm/drop_caches
	fi
	[ -e /proc/slabinfo ] && cp /proc/slabinfo "$RESULT_BASE/slabinfo.after"
	cp /proc/meminfo "$RESULT_BASE/meminfo.after"
	free -m
	gce_run_hooks fs-config-end $TC
	umount "$TEST_DIR" >& /dev/null
	umount "$SCRATCH_MNT" >& /dev/null
	clear_pool_devs
	if test -n "$RUN_ONCE" ; then
	    cat /run/fstest-config >> "$RESULTS/fstest-completed"
	fi
	echo -n "END TEST: $TESTNAME " ; date
	logger "END TEST $TC: $TESTNAME "
done

if test -n "$FSTESTSTR" ; then
    [ -e /proc/slabinfo ] && cp /proc/slabinfo "$RESULTS/slabinfo.stress"
    cp /proc/meminfo "$RESULTS/meminfo.stress"
    systemctl status stress
    systemctl stop stress
fi

[ -e /proc/slabinfo ] && cp /proc/slabinfo "$RESULTS/slabinfo.after"
cp /proc/meminfo "$RESULTS/meminfo.after"

/usr/local/bin/gen_results_summary $RESULTS > $RESULTS/report

echo "-------------------- Summary report"

cat $RESULTS/report

if test -n "$FSTEST_ARCHIVE"; then
    tar -C $RESULTS -cf - . | \
	xz -6e > /tmp/results.tar.xz
fi
