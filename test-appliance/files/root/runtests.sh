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
FAIL_LOOP_COUNT=4
NO_TRUNCATE=

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
    soak) shift
	export SOAK_DURATION="$1"
	;;
    no_truncate_test_files)
	NO_TRUNCATE=t
	;;
    mkfs_config) shift
	MKFS_CONFIG="$1"
	;;
    *)
	echo " "
	echo "Unrecognized option $1"
	echo " "
  esac
  shift
done

gen_version_files
if test -z "$MKFS_CONFIG" ; then
    set_mkfs_config
else
    for i in /root/fs/*/mkfs_cfg/$MKFS_CONFIG.conf ; do
	if test ! -e "$i" ; then
	    echo " "
	    echo "mkfs_config ""$MKFS_CONFIG"" does not exist!"
	    echo " "
	fi
	break
    done
fi
umount "$PRI_TST_DEV" >& /dev/null
umount "$SM_TST_DEV" >& /dev/null
if ! get_fs_config $FSTESTTYP ; then
    echo "Unsupported primary file system type $FSTESTTYP"
    exit 1
fi

if test -b "$PRI_TST_DEV" -a -z "$MKFS_CONFIG" ; then
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

if test -n "$RUN_ON_GCE"
then
    cp /usr/local/lib/gce-local.config /root/xfstests/local.config
fi

touch "$RESULTS/fstest-completed"
rm -f /run/last_logged

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

runtests_before_tests

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
			-z "$MKFS_CONFIG" -a \
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
	    */ovl|9p*|virtiofs-*) ;;
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
	echo $FS_PREFIX$FS/$TC > /run/fstest-config
	if test -n "$RUN_ONCE" && \
		grep -q "^$FS_PREFIX$FS/$TC\$" "$RESULTS/fstest-completed"
	then
	    echo "$FS_PREFIX$FS/$TC: already run"
	    /usr/local/lib/gce-logger already run
	    continue
	fi
	setup_mount_opts
	gen_version_files
	export RESULT_BASE="$RESULTS/${FS_PREFIX//:/-}$FS/results-$TC"
	echo "$RESULT_BASE" > /run/result-base
	if test ! -d "$RESULT_BASE" -a -d "$RESULTS/results-$TC" ; then
	    mkdir -p "$RESULTS/$FS"
	    mv "$RESULTS/results-$TC" "$RESULT_BASE"
	fi
	mkdir -p "$RESULT_BASE"
	copy_xunit_results
	echo FS: $FS_PREFIX$FS > "$RESULT_BASE/config"
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
	if test -n "$MKFS_CONFIG_FILE" ; then
	   echo MKFS_CONFIG: $MKFS_CONFIG >> "$RESULT_BASE/config"
	fi
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
			"/root/fs/$BASE_FSTYPE/exclude" \
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
		cat /run/version_info.cpp /tmp/exclude.cpp | \
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
	    case "$FSTYP" in
		ext2|ext3|ext4)
		    tests_regexp="ext4"
		    ;;
		*)
		    tests_regexp="$FSTYP"
	    esac
	    tests_regexp="^($tests_regexp|shared|generic|perf|selftest)"
	    ./check -n $FSTESTSET 2> /tmp/tests-to-run.stderr > /tmp/tests-to-run
	    ret="$?"
	    echo "Exit status $ret" >> /tmp/tests-to-run.stderr
	    if test "$ret" -gt 0 ; then
		echo "Failed to run ./check -n $FSTESTSET"
		cat /tmp/tests-to-run /tmp/tests-to-run.stderr > /tmp/tests-to-run.debug
		cat /tmp/tests-to-run.debug
		continue
	    fi
	    sed -e '1,/^$/d' -e '/^$/d' < /tmp/tests-to-run | \
		grep -E "$tests_regexp" | \
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
		last_test=
		if test -f "/results/preempted"
		then
		    rm -f "/results/preempted"
		    if test -f "$RESULT_BASE/completed"
		    then
			# Backup and restart the test that was interrupted
			head -n -2 "$RESULT_BASE/completed" > /tmp/completed
			mv /tmp/completed "$RESULT_BASE/completed"
		    fi
		    last_test="preempted"
		elif test -f "$RESULT_BASE/completed"
		then
		    last_test="$(tail -n 1 "$RESULT_BASE/completed")"
		else
		    touch "$RESULT_BASE/completed"
		fi
		if test -n "$last_test"
		then
		    record_test_error "$last_test"
		fi
		/root/xfstests/bin/syncfs $RESULT_BASE
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
	    rm -f "$RESULT_BASE/completed"
	    umount "$TEST_DEV" >& /dev/null
	    check_filesystem "$TEST_DEV" >& $RESULT_BASE/fsck.out
	    if test $? -gt 0 ; then
		cat $RESULT_BASE/fsck.out
	    fi
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

runtests_after_tests

/usr/local/bin/gen_results_summary $RESULTS \
	--merge_file /tmp/results.xml \
	--output_file $RESULTS/report \
	--check_failure

echo "-------------------- Summary report"

cat $RESULTS/report

if test -z "$NO_TRUNCATE" ; then
    /usr/local/bin/truncate-test-files "$RESULTS"
fi

runtests_save_results_tar
exit_code=0
if [ $(/usr/local/bin/get_error_count /tmp/results.xml) -gt 0 ]; then
    exit_code=1
fi
echo "$exit_code" > /tmp/retdir/exit_code
