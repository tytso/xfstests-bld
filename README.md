** **

## Gce-xfstests Project Proposal

[Final Project Video](https://youtu.be/s5kkD3z_mWc)

** **

## 1.   Vision and Goals of The Project:

The [gce-xfstests project](https://github.com/tytso/xfstests-bld) provides fast and cost-effective cloud-based regression testing for file system and kernel developers using the Google Compute Engine (GCE). gce-xfstests provides a Light GCE-Xfstests Test Manager (LTM) that runs on a micro virtual machine (VM) and is used to launch multiple test VMs (from a user-provided image) with different test configurations, and emails a report back to the user/developer who launched the test. Using GCE allows us to run multiple tests in parallel, thereby speeding up the testing process and freeing up resources on the developer's computer. Using VMs also enables us to have hermetic builds <sup id="a1">[1](#1)</sup> to ensure consistency and repeatability. 

Our goal is to create a build server so we can introduce new features such as repository monitoring for automated testing and bug finding. With a build server, instead of providing a pre-built test image, the user will be able to provide a specific kernel version (by specifying its git commit id) which will then be used to build the test image in the cloud. The build server will then communicate to the LTM that the build is complete so that it can begin the testing. We plan to implement this by extending the functionality of the LTM's existing web services framework so that in addition to launching the LTM server, we will have the option of launching a build server running on a larger build VM.

This additional functionality will allow us to implement two key features:
* Automated testing for 'watched' repositories every time there is a new commit to the kernel;
* Bisection bug finding between any two commits given by the user, in order to identify the offending commit;

Note: <b id="1">1</b> [Hermetic builds](https://landing.google.com/sre/sre-book/chapters/release-engineering/) are insensitive to the libraries and other software installed on the build machine. Instead, builds depend on known versions of build tools, such as compilers, and dependencies, such as libraries. [↩](#a1)

## 2. Users/Personas of The Project:

The target users of the project are Linux kernel or file system developers. They can be broadly grouped as follows:
* Kernel/fs developers: these users require consistent and repeatable automated testing that is the same across builds. In addition, since they may have to run many of these tests, the testing should be reasonably fast, cheap and not hog the resources of the user's machine.
* Repository maintainers/managers: these users may want to verify that any new changes pushed to them have passed the comprehensive xfstests before pushing them further upstream for integration.
* Academic researchers: these users benefit from testing their prototypes against real-world integration tests.
* Developers in general (who care the Linux kernel or file systems).  
  
It doesn’t target:
* Non-Linux kernel developers
* Linux kernel users
* Anyone that doesn't believe in release automation

Since the developer or repository maintainer is the one launching the tests, they will incur the cost of the testing and therefore it should be as low as possible.

** **

## 3.   Scope and Features of The Project:

The two main features we are aiming to deliver are specified clearly by our mentor below:

  "The first is that the LTM server can watch a particular git repository's branch every N minutes, and if it has changed, it will fetch the newly updated kernel, and run a suite of tests against that kernel and a report sent back to the developer.

  The second feature the Build VM will enable is the ability to do automated bisection for bug finding, using the git bisect feature.   In this mode, the LTM server will be given a starting good commit and a starting bad commit, and a specific test to be run.   It will then launch the Build VM, and use the git bisect feature to find successful kernel versions to be tested so that the first bad commit which introduced the problem can be found."

gce-xfstests currently supports testing for all major file systems on Linux (xfs, ext2/3/4, cifs, btrfs, f2fs, reiserfs, gfs2, jfs, udf, nfs, tmpfs). The LTM server uses a Debian "Buster" 10 image. We plan to use this same image for the build server but it will have to be enhanced to include packages necessary for the kernel builds.

Reporting of test results is limited to an email summary but can be extended to include test failures on regression/flaky tests as a stretch goal.

Other work outside the scope of the project includes enhancing the speed of the LTM server launch, or any feature development for the closely related kvm-xfstests and the android-xfstests.

** **

## 4. Solution Concept

#### Global Architectural Structure of the Project:

#### Overview

###### _Existing architecture_:
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/arch_pre.png)

###### _Proposed architecture (with build server)_: 

![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/arch_post.png)


![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/diagram_with_logos_v2.JPG)

#### Design Implications and Discussion:
As we are building on top of the gce-xfstests project, we will continue using the same technology stack. Below is a description of the system components that will be used to accomplish our goals:

* Google Compute Engine: Infrastructure as a Service (IaaS) used to launch virtual machines;
* Google Cloud Storage:  used to store kernel image under test;
* Virtual Machine: bucket used for building the kernel and running tests;
* Lightweight GCE-Xfstests Test Manager (LTM) server: to manage testing and the build server that we'll implement, and to monitor VMs and repositories;
* Build server: to build test images from given kernel source code;
* Flask: a lightweight web application framework for LTM and build server to establish web services for communicating between VMs and with users
* Git: to fetch a specific kernel, supervise new kernel commits and assist with the bisection bug finding feature;
* JUnit-XML: library used to generate the test result report;
* SendMail: used to send the result report by SMTP mail server;

Relevant git commands to implement the features: 
* `git remote update`, `git fetch` and `git status` to see whether a watched git repository is updated and should be fetched;
* `git bisect` to find the commit that introduced a bug via binary search.


###### _Build server_
This stage will be completed first and consists of three parts:

+ We will use the existing web services framework (Flask) to launch the build VM and communicate between it and the LTM. This will allow us to reuse a lot of the existing code to complete this part quickly and cleanly. So instead of running `gce-xfstests launch-ltm` we can instead run `gce-xfstests launch-bldsrv` to launch the build server. Although Flask is often used for synchronous communication, we will implement asynchronous communication between the build server and the LTM by using two synchronous endpoints.

+ The build server will need a larger VM than the micro VM used for the LTM server. The exact size and details will be worked out based on cost trade-offs.

+ Instead of using a separate image for the build VM, we will enhance the current Debian image to include packages needed to build the kernel (such as make, gcc, etc.)

###### _Repository monitoring_
Here the LTM server is the long-running server and is responsible for monitoring the repository. When it detects a change, it launches a build server and instructs it to build an image from the new commit. The first step to implementing this will be to allow the user to pass a git commit id as a command-line argument when running gce-xfstests. That should build the image from the commit rather than upload a pre-built image from the user.

To enable repository monitoring and testing, we can take one of two approaches:

+ keep the build server alive between builds 

+ shutdown the build server in between builds

It makes sense to keep the build server running in between builds so that we can take advantage of the existing build tree to minimize build time after small changes to the kernel. However, this approach incurs a large storage cost that needs to be balanced against the cost of shutting down the server between builds and building the kernel from scratch every time. We will most likely take the first approach, however, we may revise this as we are further along in the project

![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature1.png)  

###### _Bisection testing_
The main hurdle here will be figuring out where to store the whole git tree which is needed for git bisect to work. It makes sense for it to be on the build server but then the LTM will not be able to access it. We will need a way for the LTM to communicate to the build server which version of the kernel to build. Perhaps we can set up some mechanism where the build server decides which commit to using for the build based on feedback from the LTM server. For example, the LTM server reviews the results of the tests and then instructs the build server to keep bisecting the git tree and building kernels till the bug is found. We are not clear on all of the details of this part at the moment, but we will revise this section as our understanding of the problem improves.
![](https://github.com/BU-NU-CLOUD-F19/gce-xfstests/blob/master/Pictures/feature2.png)  


###### _Other_
The Google Cloud SDK is used to build the GCE image from the root disk of the Debian build VM thus speeding up build time and ensuring that builds are reliable and reproducible. Since our goal is to be able to run automated tests every N minutes, it is essential to rely on a cloud VM so as not to overwhelm the user's machine. In addition, we are able to leverage GCE to launch multiple parallel VM instances to significantly speed up the testing process (currently ~7/8 hours)

The SendMail integration is a very practical and efficient way to report backtest results to the user, but we are open to considering other approaches to organizing and sharing the test results especially given the increased frequency of automated testing. However, this will largely depend on whether there is a user need for such functionality.

## 5. Acceptance criteria

(We modified our minimum acceptance criteria in Sprint 2 since we realized the amount of work needed to implement the build server and to enable the asynchronous communication between it and the LTM)

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

In Sprint 2, we made the following accomplishments:

* Add build tools to Debian image (use by LTM & build server)
* Use Flask to communicate between two GCE machines
* Launch & kill build server that does nothing
* Wrapper for gcloud compute scp to make testing easier

*Presentation*

[Sprint 2 presentation](https://docs.google.com/presentation/d/1wt8tg2oTK2Bcez4L0uwRLAbNVBEXQibzkAu_S9qRJSg/edit?usp=sharing)

**Sprint 3: 10/10 – 10/24**

*Sprint Progress*

In Sprint 3, we made the following accomplishments:

* Use build server to build kernel
* Use build server to build kernel from git commit
* Start two-way communication between LTM & build server 
*	User story: User wants to run tests without using resources on their local machine to build the kernel.  User is able to build the kernel using the build server, and specify the commit.

*Presentation*

[Sprint 3 presentation](https://docs.google.com/presentation/d/1H4hqiuCrjCaMHDIxIzGvnmgX5nTJR4KF1Rgc1sRTMUI/edit?usp=sharing)

**Sprint 4: 10/24 – 11/07**

*Sprint Progress*

In Sprint 4, we made the following accomplishments:

* Use ltm to launch build server
* Added error handling, logging, and testing to existing code
* Continue two-way communication between LTM & build server 

*Presentation*

[Sprint 4 presentation](https://docs.google.com/presentation/d/13IO25TCbVnzGfY53VFzKXKWx0aUPtIrSCe5YqINw8Ng/edit?usp=sharing)

**Sprint 5: 11/07 – 11/26**

*Sprint Progress*

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

2) How do we test repository monitoring? In particular, how do we simulate new commits to a repository so that we can build and test new versions? We thought of using existing commits to simulate new commits, but the details on this are fuzzy at the moment.

3) At some point, should we move away from Flask? Moving away from it might simplify migration from python2 to python3, and allows us to go back to using a smaller VM (f1) for LTM

4) For git bisect, how to ignore bugs that are NOT the ones we are looking for. This is a problem that comes up in practice.


** **

## 8.  Contributors:

* [Gordon Wallace](https://github.com/GordonWallace)
* [Jing Li](https://github.com/jingli18)
* [Maha Ashour](https://github.com/mashbu)
* [Zhenpeng Shi](https://github.com/ZhenpengShi)

** **

## 9. Other notes:

You can find the presentations for each sprint at the end of the sprint's details in the release planning. Or [here]( https://docs.google.com/presentation/d/1y-bZKlb2oy8LZN8qLJX1udenXNFqnqrlBj7uVAU5JZA/edit?usp=sharing).
