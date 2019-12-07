#!/bin/bash
# This script conditionally boots the test appliance into the light test
# manager.

. /usr/local/lib/gce-funcs

gcloud config set compute/zone $ZONE

fstesttz=$(gce_attribute fstesttz)
if test -n "$fstesttz" -a -f /usr/share/zoneinfo/$fstesttz
then
    ln -sf /usr/share/zoneinfo/$fstesttz /etc/localtime
    echo $fstesttz > /etc/timezone
fi

/usr/local/lib/gce-logger starting ltm server

# login shells dont need test env on the LTM (shouldn't be running tests
# in the LTM vm)
echo > ~/test-env

# here we know that we don't want to kexec, we want to boot into the webserver.
# kill the timer that would cause shutdown
systemctl stop gce-finalize.timer
systemctl disable gce-finalize.timer

logger -i "Disabled gce-finalize timer"

# Configure swap space so that we have some extra elbow room; otherwise
# some monitoring threads for ltm shards can end up dying due to memory
# allocation failures
fallocate -l 2G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile

# adjust the configuration of the web server of the test appliance and
# relaunch lighttpd with the new configuration.
# essentially create a webserver right here.

if test ! -d "/var/log/lgtm"
then
    # we only want to do this if the server isn't set up already.
    # (e.g. if the ltm server instance was stopped and restarted, this should
    # not get executed)
    rm -r /var/www/*
    mkdir -p /var/log/lgtm
    chown www-data:www-data -R /var/log/lgtm
    lighttpd-enable-mod fastcgi
    cat /usr/local/lib/gce-ltm/ltm-lighttpd.conf >> /etc/lighttpd/lighttpd.conf
    # Webserver static files should go in static
    mv /usr/local/lib/gce-ltm/static /var/www/
    systemctl restart lighttpd.service
    # Restart to allow conf changes to take effect.
fi
