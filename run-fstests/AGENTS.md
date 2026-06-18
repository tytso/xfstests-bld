# run-fstests

This directory contains the client front-end programs and utilities
for running xfstests in various environments: QEMU/KVM
(`kvm-xfstests`), Google Compute Engine (`gce-xfstests`), and Android
devices (`android-xfstests`).

## Key Scripts

- `kvm-xfstests`: Runs tests in a local QEMU/KVM virtual machine.
- `gce-xfstests`: Runs tests in Google Compute Engine VMs. Supports distributed testing (LTM - Lightweight Test Manager) and kernel compilation (KCS - Kernel Compilation Service).
- `android-xfstests`: Runs tests on an Android device connected via USB/ADB.
