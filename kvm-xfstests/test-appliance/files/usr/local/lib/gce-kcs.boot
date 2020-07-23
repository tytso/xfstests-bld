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

if test ! -d "/var/log/go"
then
    # we only want to do this if the server isn't set up already.
    # (e.g. if the compile server instance was stopped and restarted, this should
    # not get executed)
    mkdir -p /var/log/go

    # attach cache PD with repos and ccache
    ./usr/local/lib/gce-repo-cache &> /var/log/gce-repo-cache.log

    systemctl enable gce-kcs.service
    systemctl start gce-kcs.service
fi
