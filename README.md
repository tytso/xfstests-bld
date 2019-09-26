** **

## Gce-xfstests Project Proposal

** **

## 1.   Vision and Goals Of The Project:

The [gce-xfstests project](https://github.com/tytso/xfstests-bld) provides fast and cost-effective cloud-based regression testing for file system and kernel developers using the Google Compute Engine (GCE). The project provides a Light GCE-Xfstests Test Manager (LTM) that runs on a micro virtual machine (VM), which then launches multiple test VMs with various configurations from a provided test image, and emails a report back to the user/developer. Using GCE allows us to run multiple tests in parallel, thereby speeding up the testing process and freeing up resources on the developer's computer. Using VMs also enables us to have hermetic builds to ensure consistency and repeatability. 

Our goal is to create a build server so that instead of providing a test image, the user can provide a specific kernel version (by specifying its git commit id) which will then be used to build the test image in the cloud. The build server will then communicate to the LTM that the build is complete, so that it can begin the testing. We plan to implement this by extending the functionalility of the LTM's exisiting web services framework so that instead of launching the LTM server, we will have the option of launching a build server running on larger build VM.

This additional functionality will allow us to implement two key features:
* Automated testing for 'watched' repositories every time there is a new commit to the kernel;
* Bisection bug finding between any two commits given by the user, in order to identify the offending commit;


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

** **

## 3.   Scope and Features Of The Project:

The two main features we are aiming to deliver are specified clearly by our mentor below:

  "The first is that the LTM server can watch a particular git repository's branch every N minutes, and if it has   changed, it will fetch the newly updated kernel, and run a suite of tests against that kernel and a report sent back to the developer.

  The second feature the Build VM will enable is the ability to do automated bisection for bug finding, using the   git bisect feature.   In this mode, the LTM server will be given a starting good commit, and a starting bad commit, and a specific test to be run.   It will then launch the Build VM, and use the git bisect feature to find successful kernel versions to be tested, so that the first bad commit which introduced the problem can be found."

The gce-xfstests currently supports all major file systems on Linux (xfs, ext2/3/4, cifs, btrfs, f2fs, reiserfs, gfs2, jfs, udf, nfs, tmpfs). The build VM uses a Debian "Buster" 10 image. 

Reporting of test results is limited to an email summary, but can be extended to include test failures on regression/flaky tests as a stretch goal.

Other work outside the scope of the project includes enhancing the speed of the LTM server launch, or any feature development for the closely related kvm-xfstests and the android-xfstests.

** **

## 4. Solution Concept

#### Global Architectural Structure of the Project:

#### Overview

Existing architecture:
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/old_arch.png)

Propose architecture (with build server): 

![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/new_build_diagram.JPG)
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/new_arch.png)

Repository monitoring:

![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature1.png)  

Bisection bug finding:

![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature2.png)  

#### Design Implications and Discussion:
As we are building on top of the gce-xfxtests project, we will continue using the same techonology stack. Below is a description of the system components that will be used to accomplish our goals:

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
This stage will be completed first and consists of two parts:

+ We will use the existing web services framework to launch the build VM and communicate between it and the LTM. This will allow us to reuse a lot of the existing code to complete this part quickly and cleanly. 

+ Instead of using a separate image for the build VM, we will enhance the current Debian image to include packages needed to build the kernel (such as make, gcc, etc.)

###### _Repository monitoring_
To enable repository monitoring and testing, we can take one of two approaches:

+ keep the build server alive between builds 

+ shutdown the build server inbetween builds

It makes sense to keep the build server running in between builds so that we can take advantage of the existing build tree to minimize build time after small changes to the kernel. However this approach incurs a large storage cost that needs to be balanced against the cost of shutting down the server between builds and building the kernel from scratch every time. We will mostly likely take the first approach, however we may revise this as we are further along in the project

###### _Bisection testing_
The main hurdle here will be figuring out where to store the whole git tree which is needed for git bisect to work. It makes sense for it to be on the build server but then the LTM will not be able to access it. We will need a way for the LTM to communicate to the build server which version of the kernel to build. Perhaps we can set up some mechanism for the build server to decide. We are not sure how to approach this right now, but we will revise this section as our understanding of the problem improves.


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
