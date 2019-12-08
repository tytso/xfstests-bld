** **

## Gce-xfstests Project Proposal

[Final Project Video](https://youtu.be/s5kkD3z_mWc)

** **

## 1.   Vision and Goals of The Project:

The [gce-xfstests project](https://github.com/tytso/xfstests-bld) provides fast and cost-effective cloud-based regression testing for file system and kernel developers using the Google Compute Engine (GCE). A user can initiate tests directly for a kernel under test, or using the Light GCE-Xfstests Test Manager (LTM). The LTM runs on a micro virtual machine (VM) and oversees the testing process. It can launch multiple test VMs with different test configurations, and emails a report back to the user/developer who launched the test. Using GCE allows us to run multiple tests in parallel, thereby speeding up the testing process and freeing up resources on the developer's computer. Using VMs also enables us to have hermetic builds <sup id="a1">[1](#1)</sup> to ensure consistency and repeatability.

Our goal is to contribute to the project by developing a build server that can be used to implement new features such as repository monitoring for automated testing and bisection bug finding. With a build server, instead of building the kernel under test locally, the user will be able to provide a specific kernel version (by specifying its git commit id) which will then be used to build the kernel in the cloud. The build server can be used as a standalone service or to execute builds on behalf of the LTM. In the latter scenario, the LTM has to communicate to the build server the specific build request, and the build server has to notify the LTM when the build is complete so that it can begin the testing. We plan to implement this by extending the functionality of the LTM's existing web services framework so that in addition to launching the LTM server, we will have the option of launching a build server running on a larger build VM.

This additional functionality will allow us to implement two key features:
* Automated testing for 'watched' repositories every time there is a new commit to the kernel;
* Bisection bug finding between any two commits given by the user, in order to identify the offending commit;

Note: <b id="1">1</b> [Hermetic builds](https://landing.google.com/sre/sre-book/chapters/release-engineering/) are insensitive to the libraries and other software installed on the build machine. Instead, builds depend on known versions of build tools, such as compilers, and dependencies, such as libraries. [↩](#a1)

## 2. Users/Personas of The Project:

The target users of the project are Linux kernel or file system developers. They can be broadly grouped as follows:
* Kernel/fs developers: these users require consistent and repeatable automated testing that is the same across builds. In addition, since they may have to run many of these tests, the testing should be reasonably fast, cheap and not hog the resources of the user's machine.
* Repository maintainers/managers: these users may want to verify that any new changes or pull requests have passed the comprehensive xfstests before integrating them into the upstream.
* Academic researchers: these users benefit from testing their prototypes against real-world integration tests.
* Developers in general (who care the Linux kernel or file systems).  

It doesn’t target:
* Non-Linux kernel developers
* Linux kernel users
* Anyone that doesn't believe in continuous integration and release automation

Since the developer or repository maintainer is the one launching the tests, they will incur the cost of the testing and therefore it should be as low as possible.

** **

## 3.   Scope and Features of The Project:

Our *minimum viable product (MVP)* is a build server that meets the criteria below:

  1. The LTM can launch the build server and request a build.
  2. The build server can build a kernel from a repository, commit (tag or branch) and user-supplied defconfig file.
  3. The build server can notify the LTM that the build is complete.
  3. The LTM can shut down the build server if it is not needed.

Beyond that, the two main features we are aiming to deliver are specified clearly by our mentor below:

  "The first is that the LTM server can watch a particular git repository's branch every N minutes, and if it has changed, it will fetch the newly updated kernel, and run a suite of tests against that kernel and a report sent back to the developer.

  The second feature the Build VM will enable is the ability to do automated bisection for bug finding, using the git bisect feature.   In this mode, the LTM server will be given a starting good commit and a starting bad commit, and a specific test to be run.   It will then launch the Build VM, and use the git bisect feature to find successful kernel versions to be tested so that the first bad commit which introduced the problem can be found."

gce-xfstests currently supports testing for all major file systems on Linux (xfs, ext2/3/4, cifs, btrfs, f2fs, reiserfs, gfs2, jfs, udf, nfs, tmpfs). The LTM server uses a Debian "Buster" 10 image. We plan to use this same image for the build server but it will have to be enhanced to include packages necessary for the kernel builds.

Other work outside the scope of the project includes enhancing the speed of the LTM server launch, or any feature development for the closely related kvm-xfstests and the android-xfstests.

** **

## 4. Solution Concept

#### Global Architectural Structure of the Project:

#### Overview

###### _Existing architecture_:
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/arch_pre.png)

###### _Proposed architecture (with build server)_:

![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/arch_post.png)

#### Design Implications and Discussion:
As we are building on top of the gce-xfstests project, we will continue using the same technology stack. Below is a description of the system components that will be used to accomplish our goals:

* Google Compute Engine: Infrastructure as a Service (IaaS) used to create and manage virtual machines and disks;
* Google Cloud Storage: used to store project files, such as the kernel under test;
* Virtual Machine: used for building kernels and running tests;
* Lightweight GCE-Xfstests Test Manager (LTM) server: to manage testing and the build server, and to monitor VMs and repositories;
* Build server: to build test kernels from given kernel source code;
* Flask: a lightweight web application framework for LTM and build server to establish web services for communicating between VMs and with users
* Git: to fetch a specific kernel, monitor new kernel commits and assist with the bisection bug finding feature;
* SendMail: used to send the result report by SMTP mail server;

Relevant git commands to implement the features:
* `git remote update`, `git fetch` and `git status` to see whether a watched git repository is updated and should be fetched;
* `git bisect` to find the commit that introduced a bug via binary search.


###### _Build server_
This stage will be completed first and consists of four parts:

+ We will develop a standalone build server that can be use to build a kernel from a commit and store the resulting bzImage in the project's Google storage bucket. The build server should be able to gracefully handle any errors due to invalid commit ids or repository urls. The build server should also log any relevant information to troubleshoot failed builds.

+ We will use the existing web services framework (Flask) to launch the build VM and communicate between it and the LTM. This will allow us to reuse a lot of the existing code to complete this part quickly and cleanly. So instead of running `gce-xfstests launch-ltm` we can instead run `gce-xfstests launch-bldsrv` to launch the build server. Although Flask is often used for synchronous communication, we will implement asynchronous communication between the build server and the LTM by using two synchronous endpoints.

+ The build server will need a larger VM than the micro VM used for the LTM server. The exact size and details need to be worked out based on cost trade-offs. Unfortunately we did not have enough time to do a full cost analysis.

+ Instead of using a separate test appliance image for the build VM, we will enhance the current Debian image to include packages needed to build the kernel (such as make, gcc, etc.)

###### _Repository monitoring_
Here the LTM server is the long-running server and is responsible for monitoring the repository. When it detects a change, it launches a build server and instructs it to build an kernel from the new commit. While we did not get to implement this portion of the project we lay out the steps necessary to complete this part.

The first step to completing this portion will be to implement a new gce-xfstests subcommand such as `gce-xfstests monitor`.

Next, this should launch the LTM and initiate a monitoring loop that polls the repository using the GitPython library. Alternatively the monitor can be implemented using a shell script.

When a new commit is detected, the ltm executes a build from that commit in the same way it would have if it were receiving the request directly from the user.

Finally the LTM emails a copy of the test report back to the user requesting the monitoring.


![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature1.png)  

###### _Bisection testing_
While we didn't complete this feature, we foresee three main challenges in its implementation.

The first is figuring out where to store the source tree which is needed for git bisect to work. It makes sense for it to be on the build server but then the LTM will not be able to access it. Part of this has already been addressed in the [Improving build time](#improving_build_time) section below, and we would likely continue using the same approach (of caching the source tree on a detachable persistent disk).

Second, we will need a way for the LTM to communicate to the build server which version of the kernel to build. Perhaps we can set up some mechanism where the build server decides which commit to use for the build based on feedback from the LTM server. For example, the LTM server reviews the results of the tests and then instructs the build server to keep bisecting the source tree and building kernels till the bug is found.

Third, we would need to identify a method for flagging only the bugs we care about or are interested in finding. One can imagine a scenario where a user is trying to find a specific bug but the bisection testing returns another unrelated bug that is not immediately relevant to the user's search. This is an important practical problem and will need careful consideration in the implementation of this feature.
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature2.png)  

###### _Improving build time_
With repeated builds (such as for repository monitoring or bug finding) it is important to speed up the repository clone/update time and the build time as well as minimize any latency due to the build server startup and shutdown time. Broadly we could take one of two approaches:

+ keep the build server alive between builds

+ shutdown the build server in between builds

Our first instinct was to keep the build server running in between builds so that we can take advantage of the existing build tree to minimize build time after small changes to the kernel. However, this approach incurs a large storage cost that needs to be balanced against the cost of shutting down the server between builds and building the kernel from scratch every time. In the end, we settled on an intermediate approach of using a regular persistent disk to cache the source tree and build objects while shutting down the server. The persistent disk would not be deleted and would instead be reattached the next time the build server is started up.

The block storage documentation for GCE states that input/output operations per second (IOPs) for a storage type are proportional to the size of the disk being used. Therefore using a larger disk gives us more IOPs, and can save time and speed up builds. However it also costs more. SSD persistent disks are faster and provide significantly more IOPs than regular persistent disks; they also cost more money. A more thoughtful analysis is need here to pick the right size and type of disk. This is something that would be important to address in future work.

## 5. Acceptance criteria

(We modified our minimum acceptance criteria in Sprint 2 since we underestimated the amount of work needed to implement the build server and to enable the asynchronous communication between it and the LTM)

Minimum acceptance criteria:
* Enhance the LTM to build a VM from a specific git commit ID.
This is the first milestone and can be verified by checking that the build matches the kernel version from the provided commit.
To achieve this goal, we need to implement a build server with the following basic behaviors:
  - Lightweight Test Manager (LTM) can launch build server
  - Build server builds a kernel from commit id passed as command line argument
  - Build server notifies LTM that it completed using web services API
  - LTM (optionally) shuts down build server if not needed anymore

Stretch goals:
* Implement the up-to-date automated kernel testing feature.
We can verify that this feature is completed if we are able to successfully receive automated testing reports every N minutes for the repository being watched.
* Implement the bisection bug finding feature. We can verify that this feature is completed if we are able to artificially introduce a bug in the kernel code and then identify the offending commit using git bisection testing.
* Provide a command-line interface for users to communicate with the LTM server and request a build of a specific git commit found on a particular git repository.
* Run regression tests and send the report to the developer if new test failures are noted.
* Run flaky tests and send the report to the developer if flaky test failures are noted.
* Reverse bisection bug finding mode: to find the first good commit when the fix to a particular problem is unknown.

## 6.  Release Planning:

**Sprint 1: 9/10 – 9/26**

*Sprint Progress*

The first sprint focused on learning the necessary technical background information needed to proceed with the project, configuring our build environments, and familiarizing ourselves with the use of the existing gce-xfstests code.  We used basic GCE commands to run a limited set of tests ("smoke" tests) both with and without the Light GCE-XFStests Manager (LTM).  We also began building our own modified images for use on the LTM, and eventually the build server.

* Technical spike: Google Compute Engine (GCE) and Light GCE-XFStests Manager (LTM).  We learned about GCE and practiced using it, particularly in conjunction with the LTM that our mentor developed.

*Presentation*

[Sprint 1 presentation](https://docs.google.com/presentation/d/10n1Gsa0CnHEb0iCrL7Uecdb7gwPbQwc_KQGtDD6IkX0/edit?usp=sharing)

**Sprint 2: 9/26 – 10/10**

*Sprint Progress*

In Sprint 2, we accomplished the following:

* Add build tools to Debian image (use by LTM & build server)
* Use Flask to communicate between two GCE machines
* Launch & kill build server that does nothing
* Wrapper for gcloud compute scp to make testing easier

*Presentation*

[Sprint 2 presentation](https://docs.google.com/presentation/d/1wt8tg2oTK2Bcez4L0uwRLAbNVBEXQibzkAu_S9qRJSg/edit?usp=sharing)

**Sprint 3: 10/10 – 10/24**

*Sprint Progress*

In Sprint 3, we accomplished the following:

* Use build server to build kernel
* Use build server to build kernel from git commit
* Start two-way communication between LTM & build server
*	User story: User wants to run tests without using resources on their local machine to build the kernel.  User is able to build the kernel using the build server, and specify the commit.

*Presentation*

[Sprint 3 presentation](https://docs.google.com/presentation/d/1H4hqiuCrjCaMHDIxIzGvnmgX5nTJR4KF1Rgc1sRTMUI/edit?usp=sharing)

**Sprint 4: 10/24 – 11/07**

*Sprint Progress*

In Sprint 4, we accomplished the following:

* Use ltm to launch build server
* Added error handling, logging, and testing to existing code
* Continue two-way communication between LTM & build server

*Presentation*

[Sprint 4 presentation](https://docs.google.com/presentation/d/13IO25TCbVnzGfY53VFzKXKWx0aUPtIrSCe5YqINw8Ng/edit?usp=sharing)

**Sprint 5: 11/07 – 11/26**

*Sprint Progress*

In Sprint 5, we accomplished the following:

* Added a persistent disk to serve as a repository cache and improve build time
* Enabled LTM to shut down build server when it is not needed
* Created prototype repository monitoring feature
* More testing

*Presentation*

[Sprint 5 presentation](https://docs.google.com/presentation/d/1LJvQpJTaI5IJ3jw9fr9J-mAuzBIP9QsBF1XzyLehLkw/edit?usp=sharing)

**Final Push: 11/26 - 12/5**

*Sprint Progress*

* Automated communication between build server and LTM
    * Previously, manual intervention at key moments required
         * LTM forwards build request to build server
         * Build server sends OK + original request back to LTM
* Submit patches to mentor for review
* User story: a developer uses the LTM to automatically launch the build server, which builds the kernel from the commit ID provided by the developer, which is then tested using test VMs launched by the LTM.

*Final Presentation*

[Final presentation](https://docs.google.com/presentation/d/1y-bZKlb2oy8LZN8qLJX1udenXNFqnqrlBj7uVAU5JZA/edit?usp=sharing)
** **

## 7. Open Questions & Risks

1) Where do we keep the git tree and how does the LTM communicate to the build server which version of the kernel to build.
  CLOSED: we decided to use a detachable persistent disk as a repository cache

2) How do we test repository monitoring? In particular, how do we simulate new commits to a repository so that we can build and test new versions? We thought of using existing commits to simulate new commits, but the details need to be worked out.

3) At some point, should we move away from Flask? Moving away from it might simplify migration from python2 to python3, and allows us to go back to using a smaller VM (f1) for LTM

4) For git bisect, how to ignore bugs that are NOT the ones we are looking for. This is a problem that comes up in practice.

** **

## 8. Instructions for installing and deploying gce-xfstests

Firstly, clone our repository:

    $ git clone https://github.com/BU-NU-CLOUD-F19/gce-xfstests.git

While our code for the gce-xfstests project can be found in the /project-code folder in **master** branch, we strongly recommend to run it on our **demo** branch to avoid unexpected problems:

    $ git checkout demo

A Google Compute Engine (GCE) project and a Google Cloud Storage (GS) bucket are needed to run our project. You can set them up following our instructions for [Method 1](#method-1-set-up-your-own-gce-project).

_If you are a professor or TA for BU EC528_, we recommend using [Method 2](#method-2-use-our-gce-project), since you have been given access to our GCE project on which you can run our code directly (if you have problems with the permission, please contact us) .

### Method 1: Set up your own GCE project

Following this documentation ([gce-xfstests](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/project-code/Documentation/gce-xfstests.md)) to complete the following steps:


1. Get a Google Compute Engine account. This step will also get you through creating your own GCE project and GS bucket.

2. Install the gce-xfstests script. In this step, you don't need to clone the xfstests-bld git repository, instead, you will install the gce-xfstests script from our **demo** branch.

3. Install the software needed by gce-xfstests.

4. Configure gce-xfstests. In the config file, you need to specify a local kernel to be uploaded and tested, which is not required by our implementation but is necessary for gce-xfstests setup. To build the kernel to be tested, you need to use the kernel defconfig files from [the kernel-configs folder](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/tree/demo/kernel-configs). When our code runs, you will notice that a new kernel is uploaded to your GS bucket after it is built by the build server. Also, you need to specify the repository from which you want our build server to build kernels, for example, add the following to the config file

       $ GIT_REPO=https://git.kernel.org/pub/scm/linux/kernel/git/tytso/ext4.git/

5. Get access to the file system test appliance.

6. Run "gce-xfstests setup".

A guide to the commands of gce-xfstests can also be found in the above page.

After setting up gce-xfstests on your computer, the next step is to create the image for the LTM and the build server from our code. Please follow this documentation ([building-xfstests](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/project-code/Documentation/building-xfstests.md)) starting from the section "choosing a build type". In the end, you will find a new file "xfstests.tar.gz".

Next, the following environment variable needs to be added to your configuration file in `~/.config/gce-xfstests`

    $ GCE_IMAGE_PROJECT="$GCE_PROJECT"

Run `gce-xfstests create-image` to let your GCE project ready to use the newly built image for the LTM and the build server.

### Method 2: Use our GCE project

Instead of setting up your own GCE project, a more convenient way is to run gce-xfstests using our GCE project. We have added professors and TAs of BU EC528 to our GCE project. In this way, you only need to follow documentation in [gce-xfstests](project-code/Documentation/gce-xfstests.md) to:

1. [Get a Google Compute Engine account](project-code/Documentation/gce-xfstests.md#get-a-google-compute-engine-account). Only having an account is enough.

2. [Install the gce-xfstests script](project-code/Documentation/gce-xfstests.md#get-and-install-the-gce-xfstests-script). Again, install the gce-xfstests script from our **demo** branch.

3. [Install the software needed by gce-xfstests](project-code/Documentation/gce-xfstests.md#install-software-required-by-gce-xfstests). When running `gcloud init` to initiating the Google Cloud SDK, set the project as `gce-xfstests-253215`, and configure the default compute region and zone as `us-central1-c` so that our GCE project can be used.

4. [Configure gce-xfstests](project-code/Documentation/gce-xfstests.md#configure-gce-xfstests). A sample config file that we recommend to use is

       GS_BUCKET=ec528-xfstests

       GCE_PROJECT=gce-xfstests-253215

       GCE_ZONE=us-central1-c

       GCE_KERNEL=

       GCE_IMAGE_PROJECT="$GCE_PROJECT"

       GIT_REPO=https://git.kernel.org/pub/scm/linux/kernel/git/tytso/ext4.git/

Now, you are ready to start using gce-xfstests for kernel building and testing.

**Build server as a standalone feature**

The build server is a standalone feature, which means you can just launch a build server and let it build the kernel without testing. To launch a build server, run

    $ gce-xfstests launch-bldsrv

If you run the command `gce-xfstests ls`, it will show a new VM called `xfstests-bldsrv` is running. Then, start a build with

    $ gce-xfstests build [--commit <commit ID or branch name>] [--config <path to defconfig>]

The kernel config file can be found in [the kernel-configs folder](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/tree/demo/kernel-configs). If you are in our project folder and on demo branch, and want to build a kernel with Linux version 5.4, an example command of building the kernel is:

    $ gce-xfstests build --commit master --config "./kernel-configs/x86_64-config-5.4"

When a new kernel is built by the build server, it will uploaded the kernel (bzImage) to the GS bucket of our GCE project. To see the files in the GS bucket, you can go to Google Cloud Console [bucket details page](https://console.cloud.google.com/storage/browser/ec528-xfstests?project=gce-xfstests-253215) or use command `gsutil ls`, for example

    $ gsutil ls -l gs://ec528-xfstests/

Note that the build server will not shut down itself, we let LTM decide whether a build server should be shut down, and if the user directly use the build server, then it's decided by the user. To shut down the build server, run

    $ gce-xfstests rm-instances xfstests-bldsrv

**Use LTM to complete kernel building and testing**

Next, we will show how to use an LTM start building a kernel by launching the build server, and then start kernel testing after kernel is uploaded to GS bucket, all done automatically with a single command from user.

To launch an LTM, run

    $ gce-xfstests launch-ltm

To test a kernel built from the repository specified in the gce-xfstests config file, type the following command

    $ gce-xfstests ltm [--commit <commit ID or branch name>] [--config <kernel-defconfig file>] [test options]

An example is

    $ gce-xfstests ltm smoke --commit master --config "./kernel-configs/x86_64-config-5.4"

The detailed explanation of test options can be found in [kvm-xfstests](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/project-code/Documentation/kvm-xfstests.md). The option "smoke" in the command above is short hand for "-c 4k -g quick".

The LTM will first launch a build server, wait for it to set up, then send the build requests to the build server for it to start building. After the kernel is built and uploaded, the build server will send the modified requests without commit and config options back to LTM, so that LTM can start testing by launching test VMs and shut down the build server. If you have set the `GCE_SG_API` and `GCE_REPORT_EMAIL` in your config file, you may receive the results as an email, if not, the testing results can be found in the /results folder in GS bucket, go to [Google Cloud Console](https://console.cloud.google.com/storage/browser/ec528-xfstests?project=gce-xfstests-253215) to download them.

**Checking the log files of LTM and build server**

When the LTM and the build server is running, you can ssh into them to see the log information, to do so, run

    $ gce-xfstests ssh xfstests-ltm
    $ gce-xfstests ssh xfstests-bldsrv

When you are in those VMs, type the command `cd /var/log/lgtm/` or `cd /var/log/bldsrv/` to locate the log files for our project in LTM and build server, respectively.

** **

## 9.  Contributors:

* [Gordon Wallace](https://github.com/GordonWallace)
* [Jing Li](https://github.com/jingli18)
* [Maha Ashour](https://github.com/mashbu)
* [Zhenpeng Shi](https://github.com/ZhenpengShi)

** **

## 10. Other notes:

You can find the presentations for each sprint at the end of the sprint's details in the release planning.
