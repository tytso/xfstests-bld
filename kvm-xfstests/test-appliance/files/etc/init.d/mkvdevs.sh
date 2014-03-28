#! /bin/sh
### BEGIN INIT INFO
# Provides:          mkvdevs
# Required-Start:    checkroot
# Required-Stop:
# Default-Start:     S
# Default-Stop:
# Short-Description: Update the /dev/vd[abcd] files
# Description:       Create the /dev/vd? device files with the 
#                    correct major number.
### END INIT INFO

PATH=/sbin:/bin:/usr/bin
. /lib/init/vars.sh
. /lib/init/tmpfs.sh

. /lib/lsb/init-functions
. /lib/init/mount-functions.sh

do_start () {

	major=$(grep virtblk /proc/devices | awk '{print $1}')

	rm -f /dev/vd? 

	mknod /dev/vda b $major 0
	mknod /dev/vdb b $major 16
	mknod /dev/vdc b $major 32
	mknod /dev/vdd b $major 48
	mknod /dev/vde b $major 64
	mknod /dev/vdf b $major 80
	mknod /dev/vdg b $major 96
	cd /dev
	for i in ttyS0 ttyS1 ttyS2 ttyS3
	do
	    if ! test -c $i ; then
		MAKEDEV $i
	    fi
	done
}

case "$1" in
  start|"")
	log_action_begin_msg "Creating virtual disk devices"
	do_start
	log_end_msg $?
	;;
  restart|reload|force-reload)
	echo "Error: argument '$1' not supported" >&2
	exit 3
	;;
  stop)
	# No-op
	;;
  *)
	echo "Usage: mkvdevs.sh [start|stop]" >&2
	exit 3
	;;
esac

