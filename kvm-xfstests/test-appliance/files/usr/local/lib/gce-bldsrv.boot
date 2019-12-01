#!/bin/bash
# This script conditionally boots the test appliance into the build server

. /usr/local/lib/gce-funcs

gcloud config set compute/zone $ZONE

fstesttz=$(gce_attribute fstesttz)
if test -n "$fstesttz" -a -f /usr/share/zoneinfo/$fstesttz
then
    ln -sf /usr/share/zoneinfo/$fstesttz /etc/localtime
    echo $fstesttz > /etc/timezone
fi

/usr/local/lib/gce-logger starting build server

# login shells dont need test env on the build sever (shouldn't be running tests
# in the build server vm)
echo > ~/test-env

# here we know that we don't want to kexec, we want to boot into the webserver.
# kill the timer that would cause shutdown
systemctl stop gce-finalize.timer
systemctl disable gce-finalize.timer

logger -i "Disabled gce-finalize timer"

if test ! -d "/var/log/bldsrv"
then
    # we only want to do this if the server isn't set up already.
    # (e.g. if the build server instance was stopped and restarted, this should
    # not get executed)
    rm -r /var/www/*
    mkdir -p /var/log/bldsrv
    chown www-data:www-data -R /var/log/bldsrv
    lighttpd-enable-mod fastcgi
    cat /usr/local/lib/gce-bldsrv/bldsrv-lighttpd.conf >> /etc/lighttpd/lighttpd.conf
    # Webserver static files should go in static
    mv /usr/local/lib/gce-bldsrv/static /var/www/
    systemctl restart lighttpd.service
    # Restart to allow conf changes to take effect.
fi
