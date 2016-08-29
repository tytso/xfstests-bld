# This Dockerfile creates an debian image with xfstests-bld build environment
#
# This is more for the sake of showing how to use a Dockerfile to
# build the xfstests.tar.gz file more than anything else.  (The resulting
# docker image is almost twice as big as it would be if we didn't try
# building it inside Docker.)
#
# VERSION 0.1
FROM debian

# Install Dependencies
RUN apt-get update && \
    apt-get install -y \
	    curl \
	    qemu-kvm \
	    qemu-utils && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* \
       /usr/share/doc /usr/share/doc-base \
       /usr/share/man /usr/share/locale /usr/share/zoneinfo

MAINTAINER Theodore Y. Ts'o tytso@mit.edu

COPY . /usr/local/lib/kvm-xfstests

RUN cd /usr/local/lib/kvm-xfstests && \
    mkdir -p /usr/local/bin && \
    sed -e 's;@DIR@;/usr/local/lib;' < kvm-xfstests.in > /usr/local/bin/kvm-xfstests && \
    chmod +x /usr/local/bin/kvm-xfstests && \
    cd test-appliance && \
    ln -s /linux /root/linux && \
    if ! test -f root_fs.img ; then \
        curl -o root_fs.img \
	    https://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests/root_fs.img.i386 ; \
    fi

# This is build environment so there is no sane default command here,
# this command simply demonstrate that the environment is sane
CMD curl -o /tmp/initrd.img https://dl.fedoraproject.org/pub/fedora/linux/releases/24/Server/i386/os/images/pxeboot/initrd.img && \
    curl -o /tmp/vmlinuz https://dl.fedoraproject.org/pub/fedora/linux/releases/24/Server/i386/os/images/pxeboot/vmlinuz && \
    kvm-xfstests --kernel /tmp/vmlinuz \
                    --initrd /tmp/initrd.img smoke
