# Release automation scripts for xfstests-bld

This directory contains the helper scripts used by the maintainer to
do kvm-xfstests release.  The procedure for doing a release is:

1.  From the top-level directory run: `./selftests/appliance`.  (This
    assumes that you have already run `./selftests/build-kernel` as
    described in the README.md file in the selftests directory.)
    Running ./selftests/appliance will build the test appliance for
    the arm64, i386, and amd64 platforms, and run basic validation tests.

2.  Then run `./release/snapshot-release` to copy the built artifacts
    into ./release/out_dir.  This script will warn if there are any
    missing files, or if the git-versions file is older than the other
    built-artifacts; this is a sign that running
    `./selftests/appliance` may have been skipped.  Check the README
    file in the out_dir file to make sure it looks valid.

3.  After verifying that the files in the out_dir directory are
    correct, then run the script `./release/upload-to-korg`.
