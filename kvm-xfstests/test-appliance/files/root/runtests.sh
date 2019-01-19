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
if test -f /var/www/cmdline
then
    echo "CMDLINE: $(cat /var/www/cmdline)" >> "$RUNSTATS"
fi
if test -n "$RUN_ON_GCE"
then
    cp /usr/local/lib/gce-local.config /root/xfstests/local.config
    . /usr/local/lib/gce-funcs
    image=$(gcloud compute disks describe --format='value(sourceImage)' \
		${instance} | \
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
    GCE_ID=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/id" -H "Metadata-Flavor: Google" 2> /dev/null)
    echo GCE ID:    \"$GCE_ID\" >> "$RUNSTATS"
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

[ -e /proc/slabinfo ] && cp /proc/slabinfo "$RESULTS/slabinfo.before"
cp /proc/meminfo "$RESULTS/meminfo.before"

if test -n "$FSTESTSTR" ; then
    systemctl start stress
fi

while test -n "$FSTESTCFG"
do
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
	echo SCRATCH_MNT: $SCRATCH_MNT >> "$RESULT_BASE/config"
	show_mkfs_opts >> "$RESULT_BASE/config"
	show_mount_opts >> "$RESULT_BASE/config"
	if test "$TEST_DEV" != "$PRI_TST_DEV" ; then
	    format_filesystem "$TEST_DEV" "$(get_mkfs_opts)"
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
	    if test -f "/root/fs/global_exclude" ; then
		sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
		    < "/root/fs/global_exclude" > "$RESULT_BASE/exclude"
	    else
		cp /dev/null "$RESULT_BASE/exclude"
	    fi
	    if test -f "/root/fs/$FS/exclude" ; then
		sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
		    < "/root/fs/$FS/exclude" >> "$RESULT_BASE/exclude"
	    fi
	    if test -f "/root/fs/$FS/cfg/$TC.exclude"; then
		sed -e 's/#.*//' -e 's/[ \t]*$//' -e '/^$/d' \
		    < "/root/fs/$FS/cfg/$TC.exclude" >> "$RESULT_BASE/exclude"
	    fi
	    if test $(stat -c %s "$RESULT_BASE/exclude") -gt 0 ; then
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
	for j in $(seq 1 $RPT_COUNT) ; do
	    gce_run_hooks pre-xfstests $TC $j
	    if test -n "$RUN_ONCE" ; then
		if test -f "$RESULT_BASE/completed"
		then
		    head -n -2 "$RESULT_BASE/completed" > /tmp/completed
		    mv /tmp/completed "$RESULT_BASE/completed"
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
		bash ./check -R xunit -T $EXTRA_OPT $AEX $TEST_SET_EXCLUDE \
		     $(cat /tmp/tests-to-run)
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
