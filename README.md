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
* git remote update and git status to see whether a watched git repository is updated and should be fetched;
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

Release planning section describes how the project will deliver incremental sets of features and functions in a series of releases to completion. Identification of user stories associated with iterations that will ease/guide sprint planning sessions is encouraged. Higher level details for the first iteration is expected.

** **


