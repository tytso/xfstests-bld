# Running xfstests on Google Compute Engine

Before running on Google Compute Engine for the first time, you will
need to do a number of setup tasks:

* Get a Google Compute Engine account
* Install the gce-xfstests script
* Install the software needed by gce-xfstests
* Get access to the File system test appliance
* Run "gce-xfstests setup"

## Get a Google Compute Engine account

If you don't have GCE account, you can go to https://cloud.google.com
and sign up for a free trial.  This will get you $300 worth of credit
which you can use over a 60 day period (as of this writing).  Given
that a full test for ext4 costs around $1.50, and a smoke test costs
pennies, that should be enough for plenty of testing.  :-)

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
bucket, but it is strongly advisable that you use a dedicated bucket
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

## Get and install the gce-xfstests script

The gce-xfstests and its associated helper scripts are part of the
xfstests-bld git repository.  If you have not fetched it, you will
need to do so now:

        git clone git://git.kernel.org/pub/scm/fs/ext2/xfstests-bld.git fstests

The gce-xfstests driver script needs to be customized so it can find
the "real" gce-xfstests script, which is located in
fstests/kvm-xfstests/gce-xfstests.   To do this:

        cd fstests
        make gce-xfstests.sh

And then copy this script to a directory in your PATH.  For example,
if ~/bin is in your shell's search path:

        cp gce-xfstests.sh ~/bin/gce-xfstests

## Install software required by gce-xfstests

1.  Install the Google Cloud SDK.  Instructions for can be found at:
https://cloud.google.com/sdk/docs/quickstart-linux

2.  Install the following packages (debian package names
used):

        % sudo apt-get install jq xz-utils dnsutils python-crcmod

## Configure gce-xfstests

You will need to set up the following configuration parameters in
~/.config/gce-xfstests:

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

Other optional parameters include:

* GCE_FIREWALL_RULES
  * List of firewall rules to add to the GCP project if not already
    present.  By default a rule "allow-http" is created which makes
    the gce-xfstests web interface accessible to anyone over the
    Internet.  It may be useful to override this if you want to
    implement more restrictive firewall rules or disable access to the
    web interface entirely.  Note that existing firewall rules
    associated with the GCP project will not be removed, and by
    default there is a default-allow-ssh rule which allows SSH access.
* GCE_USER
  * Optional identifier for all test instance names. By default, if
    this is unset, test instances will be named
    "xfstests-USER-DATECODE". (USER will be the evaluation of $USER)
    This option can be set to the empty string,
    i.e. GCE_USER= or GCE_USER="" to disable having "$USER-" in
    instance names, and simply have them named "xfstests-DATECODE"
* GCE_UPLOAD_SUMMARY
  * If set to a non-empty string value, test appliances will upload
    a summary.*.txt file in addition to the regular results tarball.
    This summary file will be a copy of the summary file normally
    found at the root directory of the results tarball.
* BUCKET_SUBDIR
  * Optional parameter to specify the subdirectory to be used to
    upload results instead of the default results/ directory.
    e.g. BUCKET_SUBDIR="4.13-rc5" or BUCKET_SUBDIR="my_subdir"
* GCE_MIN_SCR_SIZE
  * Optional value to use as a minimum scratch disk size. Must be a
    number between 0 and 250. If specified, the scratch disk created
    by any test appliances will have this value as a minimum size
    in GB. This is useful for particularly IO-bound tests (e.g.
    generic/027), which will run faster with a larger disk size.
    This is because GCE assigns IOPS per GB, so a larger scratch disk
    will have more IOPS available to it.
* GCE_LTM_KEEP_DEAD_VM
  * Optional string. If specified as a non-empty string, the LTM
    instance will preserve VMs that are presumed to have wedged/timed
    out rather than deleting the VM.


An example ~/.config/gce-xfstests might look like this:

        GS_BUCKET=tytso-xfstests
        GCE_PROJECT=tytso-linux
        GCE_ZONE=us-central1-c
        GCE_KERNEL=/build/ext4-64/arch/x86/boot/bzImage

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

If you don't want to use the pre-built image, please see the section
"Creating a new GCE test appliance image" below for instructions to
build your own image from source.

## Run "gce-xfstests setup"

The command "gce-xfstests setup" will set some GCE settings for
gce-xfstests, but more importantly, it will sanity check the
configuration parameters for gce-xfstests.  If there are any problems
or potential problems, it will report them so you can fix them.

# Running gce-xfstests

Once you have completed all of the set up tasks listed above, you can
now start using gce-xfstests.  The GCE_KERNEL configuration parameter
should be set to the location of your build directory or the kernel
that you want to boot.  So for example, you could set it to
/build/ext4, or /build/ext4/arch/x86/boot/bzImage.  If gce-xfstests is
run from the top-level of a kernel build or source tree where there is
a built kernel, gce-xfstests will use it.  Otherwise, it will use the
kernel specified by the GCE_KERNEL variable.

The design of gce-xfstests allows you to to apply a patch to your
kernel, build it, and then run "gce-xfstests smoke", which will test
the kernel without needing to install it first; gce-xfstests will
upload it to Google Cloud Storage, and then the test appliance VM will
kexec into that kernel.  This speeds up your edit, compile, debug
cycle, so you can improve your development velocity.

Running gce-xfstests is much like [kvm-xfstests](kvm-xfstests.md):

	gce-xfstests [-c <cfg>] [-g <group>]|[<tests>] ...

As with kvm-xfstests, you can also use "gce-xfstests smoke" and
"gce-xfstests full", to run the a quick smoke test and the full file
system regression test.  The command "gce-xfstests help" will provide
a quick summary of how tests can be run.

The gce-xfstests command also has a few other commands, some of which
are described below:

### gce-xfstests ssh INSTANCE

Remotely login as root to a test instances.  This is a
convenience shorthand for: "gcloud compute --project
GCE_PROJECT ssh root@INSTANCE --zone GCE_ZONE".

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
command for "gsutil rm gs://GS_BUCKET/RESULT_FILE".

### gce-xfstests get-results [--unpack | --summary | --failures ] RESULT_FILE

Fetch the run-tests.log file from the RESULT_FILE stored in
the Google Cloud Storage bucket.  The --summary or --failures
option will cause the log file to be piped into the
"get-results" script to summarize the log file using the "-s"
or "-F" option, respectively.  The "--failures" or "-F" option
results in a more succinct summary than the "--summary" or "-s"
option.

The --unpack option will cause the complete results directory
to be unpacked into a directory in /tmp instead.

# Creating a new GCE test appliance image

By default gce-xfstests uses the prebuilt image which is made
available via the xfstests-cloud project.  However, if you want to
build your own image, you must first build the xfstests tarball as
described in the [instructions for building
xfstests](building-xfstests.md).  Then run the command "gce-xfstests
create-image".  This will create a new GCE image with a name such as
"xfstests-201608132226" where 201608132226 indicates when the image
was created (in this case, August 13, 2016 at 22:26).

As with kvm-xfstests, if you want to include any additional Debian
packages, place them in the directory
kvm-xfstests/test-appliance/debs.  See the [documentation for building
kvm-xfstests appliances](building-rootfs.md) for more information.
Note that gce-xfstests requires packages for the amd64 architecture;
packages for other architectures will not be installed.

This image will be created as part of an image family called xfstests.
By default, when you start a test using gce-xfstests, the most
recently created image in the xfstests image family will be used.

In order to use the xfstests image family created in your GCE project
(instead of the xfstests-cloud project), add the following to your
`~/.config/gce-xfstests` configuration file after the GCE_PROJECT
variable is defined:

        GCE_IMAGE_PROJECT="$GCE_PROJECT"

Normally, the most recently created image in the xfstests image family
will be used by default.  You can however override this and use a
specific image by setting `ROOT_FS` in your `~/.config/gce-xfstests`
configuration file, or by using the -I option to gce-xfstests.  (For
example: `ROOT_FS=xfstests-201608130052`, or "gce-xfstests -I
xfstests-201608130052 smoke".)  You can also use the --image-project
command line option to override the GCE_IMAGE_PROJECT setting in your
configuration file.
