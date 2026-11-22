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
fallocate -l 4G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile

gcs_cp gs://$GS_BUCKET/zone-avoid-list /usr/local/lib/

if test ! -d "/var/log/go"
then
    # we only want to do this if the server isn't set up already.
    # (e.g. if the compile server instance was stopped and restarted, this should
    # not get executed)
    mkdir -p /var/log/go
    systemctl enable gce-ltm.service
    systemctl start gce-ltm.service

    # Allow requests to be sent to the LTM server by way of the GCS
    # bucket, for when gce-xfstests can't connect to this VM directly.
    systemctl enable gce-gcs-relay.service
    systemctl start gce-gcs-relay.service
fi

/usr/local/sbin/gce-xfstests cache-machtype-file
if ! validate_gce_project "$GCE_PROJECT"
then
    /usr/local/lib/gce-logger "invalid GCE_PROJECT '$GCE_PROJECT'; skipping ltm-rc and batch watcher"
    exit 1
fi
for _legacy_dir in ltm-rc ltm-batch ; do
    if gcs_list_direct_objects "$_legacy_dir" >/dev/null ; then
	/usr/local/lib/gce-logger "warning: legacy scripts in gs://$GS_BUCKET/$_legacy_dir/ are ignored; move them to gs://$GS_BUCKET/$_legacy_dir/$GCE_PROJECT/"
    fi
done
unset _legacy_dir
if test -z "$(gce_attribute skip-rc)"
then
    /usr/local/lib/gce-run-batch --keep --gce-dir "ltm-rc/$GCE_PROJECT"
fi
systemctl enable gce-ltm-batch-watcher.service
systemctl start gce-ltm-batch-watcher.service
