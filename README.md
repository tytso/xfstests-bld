# xfstests-bld

The xfstests-bld project was originally designed as system to make it
easy to build [xfstests](Documentation/what-is-xfstests.md) in way
that isolated it from the versions of various libraries such as
libaio, xfsprogs, that were available in a particular distribution.
It has since evolved to have three primary functions:

* [Building xfstests](Documentation/building-xfstests.md) to create a tar.gz file
* Running xfstests in a virtual machine using qemu/kvm ([kvm-xfstests](Documentation/kvm-xfstests.md))
* Running xfstests using Google Compute Engine ([gce-xfstests](Documentation/gce-xfstests.md))

More details about how to use xfstests-bld to carry out these three
functions can be found in the [Documentation
directory](Documentation/00-index.md).

If you are first getting started using xfstests, you should probably
read the [Quickstart guide](Documentation/kvm-quickstart.md) first.
If you don't know much about xfstests, you may also want to read this
[introduction to xfstests](Documentation/what-is-xfstests.md).


## License

The xfstests-bld project has been made available under the terms of
the GNU General Public License, version 2.  A copy can be found in the
file named [COPYING](COPYING) in the distribution.
