# Running xfstests on Google Compute Engine

## Getting a Google Compute Engine account

If you don't have GCE account, you can go to https://cloud.google.com
and sign up for a free trial.  This will get you $300 dollars worth of
credit which you can use over a 60 day period (as of this writing).
Given that a full test for ext4 costs around a $1.50, and a smoke test
costs pennies, that should be enough for plenty of testing.  :-)

### Setting up a GCE project

Although you can use a pre-existing project, it is a good idea to set
up a new GCE project for gce-xfstests.  To set up a GCE project, go to
the [GCE Projects page](https://console.cloud.google.com/projects),
pick a project name and then click on the blue "Create Project" button
at the top of the page.  The GCE projects namespace is a global one,
so you will need to pick something like unique, such as
"yourName-xfstests" or "yourUserName-xfstests".  After you create it,
you will need to [enable
billing](https://support.google.com/cloud/answer/6293499#enable-billing)
for your newly created project.

Next, go to [GCE Instances
page](https://console.cloud.google.com/compute/instances) in order to
enable the GCE API for your project.  You can optionally try creating
a VM instances via the web interface, or follow the quickstart
tutorial if you like, although this won't be necessary, since the
gce-xfstests command line interface will take care of starting and
stopping instances for you automatically.  If you do start up some
test instances yourself, please make a point of going to the GCE
Instances page when you are done to make sure you have shut down any
test VM's so that you don't have unexpected charges to your account.

### Setting up a GCS bucket

The gce-xfstests system needs a Google Cloud Storage (GCS) bucket to
send kernel images to be tested and to save the results from the test
runs.  If you are already using GCS you can use a pre-existing
bucket, but it is strongly adviseable that you use a dedicated bucket
for this purpose.  Detailed instructions for creating a new bucket can
be found in the [GCS
Quickstart](https://cloud.google.com/storage/docs/quickstart-console).

### Setting up a Sendgrid account

Since virtual machines in GCE aren't allowed to send connect to the
normal outgoing mail ports (in order prevent abuse by spammers), in
order to send e-mail we have to use a cloud mail service.  Using a
cloud mail service is optional --- you can wait for the test to
complete and then use the gce-xfstests ls-results and get-results
command to fetch the test results --- but it's very handy to have the
test reports show up in your inbox once they are finished.

The gce-xfstests system uses sendgrid, so if you would like to get
e-mailed reports, you will need to sign up for a free Sendgrid
account.  Sendgrid is designed for companies who want to do bulk
mailings, so the free account is good for up to 25,000 e-mails per
month --- and it's highly unlikely that you will be running more than
100 test runs per month, let alone 25,000!  It may take a day or two
for sendgrid to decide you are a not a robot spammer, so please start
the process right away while you familiarize yourself with the rest of
gce-xfstests.  To start, visit the [Sendgrid
website](http://www.sendgrid.com) and click on the "Try for Free"
button.

Once you have set up a Sendgrid account, get a new API key by going to
the url
[https://app.sendgrid.com/settings/api_keys](https://app.sendgrid.com/settings/api_keys)
and click on the blue "Create API Key" button and select "General API
Key".  Pick a name such as "gce-xfstests" and enter it into the "Name
of this key".  Then click on the Mail Send's "Full Access" bubble and
then click on the blue "Save" button.  Copy the API key that was
generated and use it to set the GCE_SG_API configuration variable in
gce-xfstests's config file.

Then go to Sendgrid's Tracking Settings web page at
[https://app.sendgrid.com/settings/tracking](https://app.sendgrid.com/settings/tracking)
and make sure all of the Tracking settings are set to inactive.  If
one of the trackers, such as Click Tracking, are enabled, click on the
down arrow in the Options column, and then click on "Off" to disable
the tracker.  This is important, because the reports are sent as plain
ASCII text, and the way sendgrid tries to translate the text report
into HTML results in something that looks really mangled if you are
using a mail client that tries to display the HTML version of an
e-mail message.

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

* GCE_SG_API
  * The Sendgrid API used to send the test report
* GCE_REPORT_EMAIL
  * The email addressed for which test results should be sent.
* GCE_REPORT_SENDER
  * The email used as the sender for the test report.  This defaults
    to the GCE_REPORT_EMAIL configuration parameter.  If the domain used
    by GCE_REPORT_EMAIL has restrictive SPF settings, and you don't have
    control over the domain used by GCE_REPORT_EMAIL, you may need to
    choose a different sender address.

An example ~/.config/kvm-xfstests might look like this:

        GS_BUCKET=tytso-xfstests
        GCE_PROJECT=tytso-linux
        GCE_ZONE=us-central1-c
        GCE_KERNEL=/build/ext4-64/arch/x86/boot/bzImage

## Installing software required for using gce-xfstests

1.  Install the Google Cloud SDK.  Instructions for can be found at:
https://cloud.google.com/sdk/docs/quickstart-linux

2.  Install the following packages (debian package names
used):

        % apt-get install jq xz-utils

## Add yourself to the gce-xfstests group

By default gce-xfstests uses the pre-built image which is made
available via the xfstests-cloud project.  In order gain access to
this image, you will need add the google account used for your GCE
project to the gce-xfstests Google Group.  To do this, go the
[gce-xfstests Google
Groups](https://groups.google.com/forum/#!forum/gce-xfstests) page and
click on the blue "Join group" button.  This group is an
announcement-only so it will not have a large number of posts.

The pre-built image will receive periodic updates, and while we try to
keep backwards compatibility, it may be that some new images may
require updating your local copy of the xfstests-bld git repository to
get the latest version of the gce-xfstests script.

If you don't want to use the pre-built image see the section "Creating
a new GCE test appliance image" below.

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

### gce-xfstests console [--port N] INSTANCE

Connect to serial port N on a test instance.  Port 1 is the serial
console; the magic sysrq key can be accessed via the Enter key,
followed by the tilde ('~') key, followed by the 'B' key.  Ports 2, 3,
and 4 will connect to a serial port with a shell running on it.  In
the future serial port #4 may be repurposed to allow a remote gdb
connection to kgdb, if the kernel under test is built with with kgdb
support.

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

## Creating a new GCE test appliance image

By default gce-xfstests uses the prebuilt image which is made
available via the xfstests-cloud project.  However, if you want to
build your own image, you must first build the xfstests tarball as
described in the [instructions for building
xfstests](building-xfstests.md).  Next, with the working directory set
to kvm-xfstests/test-appliance, run the gce-create-image script:

        % cd kvm-xfstests/test-appliance ; ./gce-create-image

The gce-create-image command creates a new image with a name such as
"xfstests-201607170247" where 20160717... is a date and timestamp when
the image was created.  This image is created as part of an image
family called xfstests, and so the most recent xfstests image is the
one that will be used by default.  In order to use the xfstests image
family created in your GCE project, you will need to add to your
configuration file the following after the GCE_PROJECT variable is
defined (to be the name of your GCE project):

        GCE_IMAGE_PROJECT="$GCE_PROJECT"

