#!/bin/bash
# This script conditionally boots the test appliance into the compile server

. /usr/local/lib/gce-funcs

gcloud config set compute/zone $ZONE

fstesttz=$(gce_attribute fstesttz)
if test -n "$fstesttz" -a -f /usr/share/zoneinfo/$fstesttz
then
    ln -sf /usr/share/zoneinfo/$fstesttz /etc/localtime
    echo $fstesttz > /etc/timezone
fi

/usr/local/lib/gce-logger starting compile server

mkdir /root/repositories
# attach repository cache disk
# chmod +x /usr/local/lib/gce-repo-cache
# /usr/local/lib/gce-repo-cache
# mount /dev/sdb /repositories

# login shells dont need test env on the compile sever (shouldn't be running tests
# in the compile server vm)
echo > ~/test-env

# here we know that we don't want to kexec, we want to boot into the webserver.
# kill the timer that would cause shutdown
systemctl stop gce-finalize.timer
systemctl disable gce-finalize.timer

logger -i "Disabled gce-finalize timer"

systemctl stop lighttpd.service
systemctl disable lighttpd.service

if test ! -d "/var/log/kcs"
then
    # we only want to do this if the server isn't set up already.
    # (e.g. if the compile server instance was stopped and restarted, this should
    # not get executed)
    mkdir -p /var/log/kcs
    systemctl enable gce-kcs.service
    systemctl start gce-kcs.service
fi
