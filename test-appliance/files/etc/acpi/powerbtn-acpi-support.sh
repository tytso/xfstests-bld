#!/bin/bash

logger powerbtn-acpi-support.sh started

preempted="$(curl "http://metadata.google.internal/computeMetadata/v1/instance/preempted" -H "Metadata-Flavor: Google")"
if test "$preempted" = "TRUE"
then
    logger "VM preempted"
    touch /results/preempted
    /sbin/poweroff
fi

touch /run/powerbtn
if test -e /usr/local/sbin/gce-shutdown
then
    logger initiating gce-shutdown from powerbtn-acpi-support.sh
    /usr/local/sbin/gce-shutdown
fi
