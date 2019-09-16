** **

## Gce-xfstests Project Proposal

** **

## 1.   Vision and Goals Of The Project:

Gce-xfstests is an integrated test server for file systems of Linux kernels which will:
* Running on a google cloud engine;
* Can provide up-to-date commit test for every new kernel;
* Can do bisection bug finding between two commits given by users;


## 2. Users/Personas Of The Project:

This project is designed for users who are concerned about the Linux kernel building, especially for the file systems works. Below are the sets of users.
* Kernel developers: launch the build VM to see if it passes tests
* Repository managers
* Developers in general( who concerned about the Linux file system)
It doesn’t target:
* Linux kernel users


** **

## 3.   Scope and Features Of The Project:

The Scope places a boundary around the solution by detailing the range of features and functions of the project. This section helps to clarify the solution scope and can explicitly state what will not be delivered as well.

It should be specific enough that you can determine that e.g. feature A is in-scope, while feature B is out-of-scope.

** **

## 4. Solution Concept

Global Architectural Structure of the Project:
The first feature: up-to-date kernel testing (supervision).


The second feature: bisection bug finding.


Design Implications and Discussion:
Below are the descriptions of the system components that will be used to accomplish the goals:
* Google Compute Engine: the platform to run the server process;
* Virtual Machine: the holder for kernel building and test;
* Lightweight GCE-Xfstests Test Manager (LTM) server: the main process to build kernels and test file systems;
* Git: to fetch specified kernel, supervise new kernel commit and assist the bisection bug finding feature;
* JUnit-XML: library used to generate the test result report;
* SendMail: used to send the result report by SMTP mail server;
Relevant git command to implement the features: 
* git remote update and git status to see whether a watched git repository is updated and should be fetched;
* git bisect to find the commit that introduced a bug via binary search.


Design Implications and Discussion:

This section discusses the implications and reasons of the design decisions made during the global architecture design.

## 5. Acceptance criteria

Minimum acceptance criteria:
* Implement the up-to-date kernel testing feature.
* Implement the bisection bug finding feature.
Stretch goals:
* Run regression tests and send the report to the developer if new test failures are noted.
* Run flaky tests and send the report to the developer if flaky test failures are noted.
* Reverse bisection bug finding: to find the first good commit when the fix to a particular problem is unknown.

## 6.  Release Planning:

Release planning section describes how the project will deliver incremental sets of features and functions in a series of releases to completion. Identification of user stories associated with iterations that will ease/guide sprint planning sessions is encouraged. Higher level details for the first iteration is expected.

** **

## General comments

Remember that you can always add features at the end of the semester, but you can't go back in time and gain back time you spent on features that you couldn't complete.

** **

For more help on markdown, see
https://github.com/adam-p/markdown-here/wiki/Markdown-Cheatsheet

In particular, you can add images like this (clone the repository to see details):

![alt text](https://github.com/BU-NU-CLOUD-SP18/sample-project/raw/master/cloud.png "Hover text")


