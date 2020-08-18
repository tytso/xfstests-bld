# LTM and KCS Server Developer Notes

These notes are for debugging the LTM and KCS servers.

## Overview

The LTM and KCS go source code is located at [gce-server/](../kvm-xfstests/test-appliance/files/usr/local/lib/gce-server) in this repo. When you use the default GCE test appliance VM image or build your own image, the source code is located at `/usr/local/lib/gce-server/`, and pre-compiled into binary file `ltm` and `kcs` at `/usr/local/lib/bin/`. They are executed when LTM or KCS server is launched respectively.

## SSH into LTM or KCS server to check logs

When something unexpected happens, you can ssh into the LTM or KCS server with command:

        gce-xfstests ssh [xfstests-ltm|xfstests-kcs]

Where `xfstests-ltm` is for LTM server and `xfstests-kcs` is for KCS server.

The log files are located at `/var/log/go/` on the server. The web server's log goes to `server.log`, while logs for each request goes to separate folders under `ltm_logs/` or `kcs_logs/`, named with testID.

## Cache PD for KCS server

KCS server uses a persistent disk to cache data in order to speed up building. This cache pd is mounted to `/cache/` on the KCS server:

* `/cache/repositories/`: cached git repos from previous build tasks.
* `/cache/log`: packed log files from previous KCS runs. KCS server shuts down itself when stays idle, and all the log files during this run are packed in a tarball here, named with the shutdown time.
* `/cache/ccache/`: caches for ccache.

## Run Server in Debug Mode

If you make changes to the go source code, the normal approach for testing is to build a new image and launch new servers with it. You can also change code on the server directly and run it in debug mode.

After you ssh onto the server, you need to stop the running process that registered as system service first, and navigate to the go source code where you can run the server manaully.

On the LTM server:

        systemctl disable gce-ltm.service
        systemctl stop gce-ltm.service
        cd /usr/local/lib/gce-server/ltm
        go run .

On the KCS server:

        systemctl disable gce-kcs.service
        systemctl stop gce-kcs.service
        cd /usr/local/lib/gce-server/kcs
        go run .

To run the server in debug mode, set `DEBUG = true` in [logging.go](../kvm-xfstests/test-appliance/files/usr/local/lib/gce-server/util/logging/logging.go) at `/usr/local/lib/gce-server/util/logging/logging.go` before you execute these commands.

In debug mode, logs are redirected to the console with human-friendly format, and KCS server will not shut down itself automatically.

Check code docs and function comments for more details about how the server works.