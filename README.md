** **

## Gce-xfstests Project Proposal

** **

## 1.   Vision and Goals Of The Project:

The gce-xfstests project provides quick and easy regression testing for file system and kernel developers by leveraging the Google Compute Engine (GCE).The GCE allows for launching tests in the cloud and in parallel, thereby speeding up the testing process and freeing up resources on the developer's computer. gce-xfstests relies on hermetic builds to ensure consistency and repeatability. The project currently offers a Light GCE-Xfstests Test Manager (LTM) that runs on a micro virtual machine (VM), launches multiple test VMs with various configurations, and emails a report back to the user/developer. 

Our goal is to extend the functionality of the LTM so that it launchs a build VM from a specific version of the kernel, as specified by the user through a git commit id.

This additional functionality will allow us to implement two key features:
* Automated testing for 'watched' repositories every time there is a new commit to the kernel;
* Bisection bug finding between any two commits given by the user, in order to identify the offending commit;


## 2. Users/Personas Of The Project:

The target users of the project are Linux kernel or file system developers. They can be broadly grouped as follows:
* Kernel/fs developers: these users require consistent and repeatible automated testing that is the same across developers. In addition, since they may have to run many of these tests, the testing should be reasonably fast, cheap and not hog the resources of the user's machine.
* Repository maintainers/managers: these users may want to verify that any new changes pushed to them have passed the comprehensive xfstests before pushing them further upstream.
* Academic researchers: these users benefit from testing their prototypes against real-world integration tests
* Developers in general (who care the Linux kernel or file systems)  
  
It doesn’t target:
* Non-linux kernel developers
* Linux kernel users
* Anyone that doesn't believe in release automation

** **

## 3.   Scope and Features Of The Project:

The two main features we are aiming to deliver are specified clearly by our mentor below:

  "The first is that the LTM server can watch a particular git repository's branch every N minutes, and if it has   changed, it will fetch the newly updated kernel, and run a suite of tests against that kernel and a report sent   back to the developer.

  The second feature the Build VM will enable is the ability to do automated bisection for bug finding, using the   git bisect feature.   In this mode, the LTM server will be given a starting good commit, and a starting bad       commit, and a specific test to be run.   It will then launch the Build VM, and use the git bisect feature to       find successful kernel versions to be tested, so that the first bad commit which introduced the problem can be     found."

The gce-xfstests currently supports all major file systems on Linux (xfs, ext2/3/4, cifs, btrfs, f2fs, reiserfs, gfs2, jfs, udf, nfs, tmpfs). The build VM uses a Debian "Buster" 10 image. 

** **

## 4. Solution Concept

#### Global Architectural Structure of the Project:

The first feature: up-to-date automated kernel testing (supervision).
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature1.png)  

The second feature: bisection bug finding.

![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature2.png)  

#### Design Implications and Discussion:
Below is a description of the system components that will be used to accomplish our goals:
* Google Compute Engine: IaaS used to launch virtual machines;
* Virtual Machine: bucket used for building the kernel and running tests;
* Lightweight GCE-Xfstests Test Manager (LTM) server: the main process to build kernels and test file systems;
* Git: to fetch a specific kernel, supervise new kernel commits and assist with the bisection bug finding feature;
* JUnit-XML: library used to generate the test result report;
* SendMail: used to send the result report by SMTP mail server;

Relevant git commands to implement the features: 
* git remote update, git fetch and git status to see whether a watched git repository is updated and should be fetched;
* git bisect to find the commit that introduced a bug via binary search.

As we are building on the existing gce-xfxtests project, we will continue using the exisiting techonology stack notably the GCE due to its many benefits.

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

## 7.  Contributors:

* Gordon Wallace
* Jing Li
* Maha Ashour
* Zhenpeng Shi

** **
