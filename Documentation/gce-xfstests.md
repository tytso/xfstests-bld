# Running xfstests on Google Compute Engine

## Getting a Google Compute Engine account

If you don't have GCE account, you can go to https://cloud.google.com
and sign up for a free trial.  This will get you $300 dollars worth of
credit which you can use over a 60 day period (as of this writing).
Given that a full test costs around a $1.50, and a smoke test costs
pennies, that should be enough for plenty of testing.  :-)

## Configuration

You will need to set up the following configuration parameters in
~/.config/kvm-xfststs:

* GS_BUCKET
  * The name of the Google Storage bucket which should be used by
    gce-xfstests.  Your Google Compute Engine account must have access
    to read and write files in this bucket.
* GCE_PROJECT
  * The name of Google Compute Engine project which should
    be used to create and run GCE instances and disks.
* GCE_ZONE
  * The name of the Google Compute Engine zone which should be used
    by xfstests. 
* GCE_KERNEL
  * The pathname to kernel that should be used for gce-xfstests
    by default.

If you have a sendgrid account, you can set the following
configuration parameters in order to have reports e-mailed to you:

* GCE_SG_USER
  * The username for the sendgrid account used to send email
* GCE_SG_PASS
  * The password for the sendgrid account used to send email
* GCE_REPORT_EMAIL
  * The email addressed for which test results should be sent.

An example ~/.config/kvm-xfstests might look like this:
```bash
GS_BUCKET=tytso-xfstests
GCE_PROJECT=tytso-linux
GCE_ZONE=us-central1-c
GCE_KERNEL=/build/ext4-64/arch/x86/boot/bzImage
```

## Creating the image

1. Install the Google Cloud SDK.

2.  Install the following packages (debian package names
used):
```bash
apt-get install jq xz-utils git autoconf2.59 libtool-bin automake libc6-dev gcc make e2fslibs-dev pkg-config gettext libpopt-dev
```

3.  make ; make tarball

4.  cd kvm-xfstests/test-appliance ; ./gce-create-image

## Running gce-xfstests

Running gce-xfstests is much like kvm-xfstests; see the README file in
this directory for more details.

The gce-xfstests command also has a few other commands:

### gce-xfstests ssh INSTANCE

Remotely login as root to a test instances.  This is a
convenience shorthand for: "gcloud compute --project
GCE_PROJECT ssh root@INSTNACE --zone GCE_ZONE".

### gce-xfstests console INSTANCE

Fetch the serial console from a test instance.  This is a
convenience shorthand for: "gcloud compute --project
GCE_PROJECT get-serial-port-output INSTANCE --zone GCE_ZONE".

### gce-xfstests ls-instances [-l ]

List the current test instances.  With the -l option, it will
list the current status of each instance.

This command can be abbreviated as "gce-xfstests ls".

The ls-gce option is a convenience command for "gcloud compute
--project GCE_PROJECT instances list --regexp ^xfstests.*"

### gce-xfstests rm-instances INSTANCE

Shut down the instance.  If test kernel has hung, it may be useful to
use "gce-xfstests console" to fetch the console, and then use
"gce-xfstests rm" and examine the results disk before deleting it.

This command can be abbreviated as "gce-xfstests rm"

### gce-xfstests abort-instances INSTANCE

this command functions much as the "gce-xfstests rm-instances"
command, except it makes sure the results disk will be deleted.

This command can be abbreviated as "gce-xfstests abort"

### gce-xfstests ls-disks

List the GCE disks.  This is a convenience command for "gcloud
compute --project "$GCE_PROJECT" disks list --regexp
^xfstests.*"

ALIAS: gce-xfstests ls-disk

### gce-xfstests rm-disks DISK

Delete a specified GCE disk.  This is a convenience command
for "gcloud compute --project "$GCE_PROJECT" disks delete DISK"

ALIAS: gce-xfstests rm-disk

### gce-xfstests ls-results

List the available results tarballs stored in the Google Cloud
Storage bucket.  This is a convenience command for
"gsutil ls gs://GS_BUCKET/results.*" (ls-results) or
"gsutil ls gs://GS_BUCKET" (ls-gcs).

### gce-xfstests rm-results RESULT_FILE

Delete a specified result tarball.  This is a convenience
command for "gsutil ls gs://GS_BUCKET/RESULT_FILE".

### gce-xfstests get-results [--unpack | --summary | --failures ] RESULT_FILE

Fetch the run-tests.log file from the RESULT_FILE stored in
the Google Cloud Storage bucket.  The --summary or --failures
option will cause the log file to be piped into the
"get-results" script to summarize the log file using the "-s"
or "-F" option, respectively.  The "--failures" or "-F" option
results in a more succint summary than the "--summary" or "-s"
option.

The --unpack option will cause the complete results directory
to be unpacked into a directory in /tmp instead.
