# Miscellaneous kernel hacking hints

The following hints may be useful to folks who are new to kernel
development.

## Using ccache

Enabling ccache can really help speed up your kernel builds, as well
as xfstests builds.  I strongly recommend it.

## Examples of using ftrace

This is an example of how to debug the lazytime feature (which is
when you mount a file system using -m lazytime using a 4.0-rcX and
later kernels).

        cd /sys/kernel/debug/tracing
        echo 1 > events/writeback/writeback_lazytime/enable
        echo 1 > events/writeback/writeback_lazytime_iput/enable
        echo "state & 2048" > events/writeback/writeback_dirty_inode_enqueue/filter
        echo 1 > events/writeback/writeback_dirty_inode_enqueue/enable
        echo 1 > events/ext4/ext4_other_inode_update_time/enable
        cat trace_pipe

The definition of the tracepoints can be found in
include/linux/trace/events.  The tracepoints used by ext4 and be found
in ext4.h, and by jbd2 in the jbd2.h files in that directory, and so
on.

For more information, please see:

* http://lwn.net/Articles/365835/
* http://lwn.net/Articles/366796/
* http://lwn.net/Articles/370423/

