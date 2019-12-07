#!/bin/bash

touch /run/powerbtn
if test -e /usr/local/sbin/gce-shutdown
then
    /usr/local/sbin/gce-shutdown
fi
