** **

## Gce-xfstests Project Proposal

** **

## 1.   Vision and Goals Of The Project:

The [gce-xfstests project](https://github.com/tytso/xfstests-bld) provides fast and cost-effective cloud-based regression testing for file system and kernel developers using the Google Compute Engine (GCE). gce-xfstests provides a Light GCE-Xfstests Test Manager (LTM) that runs on a micro virtual machine (VM) and is used to launch multiple test VMs (from a user-provided image) with different test configurations, and emails a report back to the user/developer who launched the test. Using GCE allows us to run multiple tests in parallel, thereby speeding up the testing process and freeing up resources on the developer's computer. Using VMs also enables us to have hermetic builds <sup id="a1">[1](#1)</sup> to ensure consistency and repeatability. 

Our goal is to create a build server so we can introduce new features such as repository monitoring for automated testing and bug finding. With a build server, instead of providing a pre-built test image, the user will be able to provide a specific kernel version (by specifying its git commit id) which will then be used to build the test image in the cloud. The build server will then communicate to the LTM that the build is complete, so that it can begin the testing. We plan to implement this by extending the functionalility of the LTM's exisiting web services framework so that in addition to launching the LTM server, we will have the option of launching a build server running on a larger build VM.

This additional functionality will allow us to implement two key features:
* Automated testing for 'watched' repositories every time there is a new commit to the kernel;
* Bisection bug finding between any two commits given by the user, in order to identify the offending commit;

Note: <b id="1">1</b> [Hermetic builds](https://landing.google.com/sre/sre-book/chapters/release-engineering/) are insensitive to the libraries and other software installed on the build machine. Instead, builds depend on known versions of build tools, such as compilers, and dependencies, such as libraries. [↩](#a1)

## 2. Users/Personas Of The Project:

The target users of the project are Linux kernel or file system developers. They can be broadly grouped as follows:
* Kernel/fs developers: these users require consistent and repeatable automated testing that is the same across builds. In addition, since they may have to run many of these tests, the testing should be reasonably fast, cheap and not hog the resources of the user's machine.
* Repository maintainers/managers: these users may want to verify that any new changes pushed to them have passed the comprehensive xfstests before pushing them further upstream for integration.
* Academic researchers: these users benefit from testing their prototypes against real-world integration tests.
* Developers in general (who care the Linux kernel or file systems).  
  
It doesn’t target:
* Non-linux kernel developers
* Linux kernel users
* Anyone that doesn't believe in release automation

Since the developer or repository maintainer is the one launching the tests, they will incur the cost of the testing and therefore it should be as low as possible.

** **

## 3.   Scope and Features Of The Project:

The two main features we are aiming to deliver are specified clearly by our mentor below:

  "The first is that the LTM server can watch a particular git repository's branch every N minutes, and if it has   changed, it will fetch the newly updated kernel, and run a suite of tests against that kernel and a report sent back to the developer.

  The second feature the Build VM will enable is the ability to do automated bisection for bug finding, using the   git bisect feature.   In this mode, the LTM server will be given a starting good commit, and a starting bad commit, and a specific test to be run.   It will then launch the Build VM, and use the git bisect feature to find successful kernel versions to be tested, so that the first bad commit which introduced the problem can be found."

gce-xfstests currently supports testing for all major file systems on Linux (xfs, ext2/3/4, cifs, btrfs, f2fs, reiserfs, gfs2, jfs, udf, nfs, tmpfs). The LTM server uses a Debian "Buster" 10 image. We plan to use this same image for the build server but it will have to be enhanced to include packages necessary for the kernel builds.

Reporting of test results is limited to an email summary, but can be extended to include test failures on regression/flaky tests as a stretch goal.

Other work outside the scope of the project includes enhancing the speed of the LTM server launch, or any feature development for the closely related kvm-xfstests and the android-xfstests.

** **

## 4. Solution Concept

#### Global Architectural Structure of the Project:

#### Overview

###### _Existing architecture_:
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/old_arch.png)

###### _Proposed architecture (with build server)_: 

![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/new_arch.png)
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/new_build_diagram.JPG)

#### Design Implications and Discussion:
As we are building on top of the gce-xfstests project, we will continue using the same techonology stack. Below is a description of the system components that will be used to accomplish our goals:

* Google Compute Engine: Infrastructure as a Service (IaaS) used to launch virtual machines;
* Google Cloud Storage:  used to store kernel image under test;
* Virtual Machine: bucket used for building the kernel and running tests;
* Lightweight GCE-Xfstests Test Manager (LTM) server: to manage testing, monitoring VMs and repositories;
* Build server: to build test images;
* Git: to fetch a specific kernel, supervise new kernel commits and assist with the bisection bug finding feature;
* JUnit-XML: library used to generate the test result report;
* SendMail: used to send the result report by SMTP mail server;

Relevant git commands to implement the features: 
* `git remote update`, `git fetch` and `git status` to see whether a watched git repository is updated and should be fetched;
* `git bisect` to find the commit that introduced a bug via binary search.


###### _Build server_
This stage will be completed first and consists of three parts:

+ We will use the existing web services framework to launch the build VM and communicate between it and the LTM. This will allow us to reuse a lot of the existing code to complete this part quickly and cleanly. 
So instead of running `gce-xfstests launch-ltm` we can instead run `gce-xfstests launch-bld`.

+ The build server will need a larger VM than the micro VM used for the LTM server. The exact size and details will be worked out based on cost trade-offs.

+ Instead of using a separate image for the build VM, we will enhance the current Debian image to include packages needed to build the kernel (such as make, gcc, etc.)

###### _Repository monitoring_
Here the LTM server is the long running server and is responsible for monitoring the repository. When it detects a change, it launches a build server and instructs it to build an image from the new commit. The first step to implementing this will be to allow the user to pass a git commit id as a command line argument when running gce-xfstests. That should build the image from the commit rather than upload a pre-built image from the user.

To enable repository monitoring and testing, we can take one of two approaches:

+ keep the build server alive between builds 

+ shutdown the build server in between builds

It makes sense to keep the build server running in between builds so that we can take advantage of the existing build tree to minimize build time after small changes to the kernel. However this approach incurs a large storage cost that needs to be balanced against the cost of shutting down the server between builds and building the kernel from scratch every time. We will mostly likely take the first approach, however we may revise this as we are further along in the project

![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature1.png)  

###### _Bisection testing_
The main hurdle here will be figuring out where to store the whole git tree which is needed for git bisect to work. It makes sense for it to be on the build server but then the LTM will not be able to access it. We will need a way for the LTM to communicate to the build server which version of the kernel to build. Perhaps we can set up some mechanism where the build server decides which commit to use for the build based on feedback from the LTM server. For example, the LTM server reviews the results of the tests and then instructs the build server to keep bisecting the git tree and building kernels till the bug is found. We are not clear on all of the details of this part at the moment, but we will revise this section as our understanding of the problem improves.
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature2.png)  


###### _Other_
The Google Cloud SDK is used to build the GCE image from the root disk of the Debian build VM thus speeding up build time and ensuring that builds are reliable and reproducible. Since our goal is to be able to run automated tests every N minutes, it is essential to rely on a cloud VM so as not to overwhelm the user's machine. In addition, we are able to leverage GCE to launch multiple parallel VM instances to significantly speed up the testing process (currently ~7/8 hours)

The SendMail integration is a very practical and efficient way to report back test results to the user, but we are open to considering other approaches to organizing and sharing the test results especially given the increased frequency of automated testing. However, this will largely depend on whether there is a user need for such a functionality.

## 5. Acceptance criteria

Minimum acceptance criteria:
* Enhance the LTM to build a VM from a specific git commit ID. 
This is the first milestone and can be verified by checking that the build matches the kernel version from the provided commit.

* Implement the up-to-date automated kernel testing feature. 
We can verify that this feature is completed if we are able to successfully receive automated testing reports every N minutes for the repository being watched.

* Implement the bisection bug finding feature. We can verify that this feature is completed, if we are able to artificially introduce a bug in the kernel code and then identify the offending commit using git bisection testing.

Stretch goals:
* Provide a command-line interface for users to communicate with the LTM server and request a build of a specific git commit found on a particular git repository.
* Run regression tests and send the report to the developer if new test failures are noted.
* Run flaky tests and send the report to the developer if flaky test failures are noted.
* Reverse bisection bug finding mode: to find the first good commit when the fix to a particular problem is unknown.

## 6.  Release Planning:

**Sprint 1: 9/19 – 10/3**

*Sprint Goals*

The first sprint will focus on learning the necessary technical background information needed to proceed with the project, configuring our build environments, and familiarizing ourselves with the use of the existing gce-xfstests code.  We will also take our first steps towards our first milestone, the LTM repository monitoring and testing on an automatically generated VM.

* Technical spike: Google Compute Engine (GCE) and Light GCE-XFStests Manager (LTM).  We will learn about GCE and practice using it, particularly in conjunction with the LTM that our mentor has developed.  We will practice launching VMs with various configurations and verify our results through the emailed reports.

* Begin progress towards first milestone by writing code to monitor a repository branch, detect whether there have been changes, and automatically launching a VM with the correct kernel version.

*Possible User Stories*

* User would like to monitor a repository, so they set an interval of one hour at which to receive a report on changes.

* User wants a VM with the same kernel identified in their commit, and one is generated automatically.

*Presentation*

[Sprint 1 presentation](https://docs.google.com/presentation/d/10n1Gsa0CnHEb0iCrL7Uecdb7gwPbQwc_KQGtDD6IkX0/edit?usp=sharing)

**Sprint 2: 10/3 – 10/17**

*Sprint Goals*

In Sprint 2, we’d like to move close to completion on our first milestone and begin work on our second.  Particularly, we’d like to finish the following features:

*	Implement desired filesystem tests on automatically generated VMs.

*	This will allow us to begin working on bisection bug testing, since our system will be identifying bad commits.  Accordingly, we’ll begin work on milestone two.

*Possible User Stories*

*	User wants to run a series of tests on a repository at a desired interval.  The user is able to implement those tests and run them automatically at that interval.

**Sprint 3: 10/17 – 10/31**

*Sprint Goals*

We’d like to reach the first milestone towards our minimum acceptance criteria in Sprint 3:

*	New LTM feature completed, which monitors a repository, and at an interval, builds a VM with the correct kernel based on the commit ID and runs tests on the kernel.

*	Verify completion of this by receiving accurate emailed results.

*	Continue bisection bug testing work.

*Possible User Stories*

*	User receives emailed reports on new commits to a repository and the tests results on automatically generated VMs at a designated interval.

**Sprint 4: 10/31 – 11/14**

*Sprint Goals*

In Sprint 4, we aim to complete the minimum acceptance criteria for the project.  This means that in addition to the LTM improvements from Sprint 3, we will finish the following:

*	Automated kernel testing (accurate reports being generated at a specified interval)

*	Correct identification of bad commits using bisection bug testing

*Possible User Stories*

*	Bisection bug testing allows the user to see which commit caused the failure in their emailed test results.

**Sprint 5: 11/14 – 11/28**

*Sprint Goals*

This sprint is dedicated to the completion of any goals that weren’t completed on time in previous sprints, and to reaching stretch goals.  Specifically, these goals include:

*	Checking for test failures by regression and flaky tests (and notifying the developer)

*	Finding the first good commit by reverse bisection bug testing

*Possible User Stories*

*	The user receives emailed reports on the results of these new features.



** **

## 7. Open Questions & Risks

1) Where do we keep the git tree and how does the LTM communicate to the build server which version of the kernel to build.

2) How do we test repository monitoring? In particular how do we simulate new commits to a repository so that we can build and test new versions? We thought of using existing commits to simulate new commits, but the details on this are fuzzy at the moment.

** **

## 8.  Contributors:

* [Gordon Wallace](https://github.com/GordonWallace)
* [Jing Li](https://github.com/jingli18)
* [Maha Ashour](https://github.com/mashbu)
* [Zhenpeng Shi](https://github.com/ZhenpengShi)

** **

## 9. Other notes:

You can find the presentations for each sprint at the end of the sprint's details in the release planning. Or [here](https://docs.google.com/presentation/d/10n1Gsa0CnHEb0iCrL7Uecdb7gwPbQwc_KQGtDD6IkX0/edit?usp=sharing).
