# This Dockerfile creates kvm-xfstests runtime environment
#
# For reasonable performance one needs to grant access to /dev/kvm
#
# Example usage:
# docker run --device /dev/kvm:/dev/kvm \
#            -v /my-kernel:/tmp tytso/kvm-xfstests \
#	  	 kvm-xfstests --kernel /tmp/vmlinuz smoke
#
# VERSION 0.2
FROM alpine

RUN apk add --no-cache --update \
	bash \
	e2fsprogs-libs \
	e2fsprogs \
	e2fsprogs-extra \
	curl \
	util-linux \
	qemu-img \
	qemu-system-x86_64 \
	tar

MAINTAINER Theodore Y. Ts'o tytso@mit.edu

COPY . /usr/local/lib/kvm-xfstests

RUN cd /usr/local/lib/kvm-xfstests && \
    mkdir -p /usr/local/bin && \
    sed -e 's;@DIR@;/usr/local/lib;' < kvm-xfstests.in > /usr/local/bin/kvm-xfstests && \
    chmod +x /usr/local/bin/kvm-xfstests && \
    sed -ie "s/QEMU=.*/QEMU=qemu-system-x86_64/g" config.kvm && \
    mkdir -p /logs && \
    ln -s /logs logs && \
    cd test-appliance && \
    ln -s /linux /root/linux && \
    if ! test -f root_fs.img ; then \
        curl -o root_fs.img \
	    https://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests/root_fs.img.i386 ; \
    fi

# This is build environment so there is no sane default command here,
# this command simply demonstrate that the environment is sane
CMD curl -o /tmp/initrd.img https://dl.fedoraproject.org/pub/fedora/linux/releases/26/Server/x86_64/os/images/pxeboot/initrd.img && \
    curl -o /tmp/vmlinuz https://dl.fedoraproject.org/pub/fedora/linux/releases/26/Server/x86_64/os/images/pxeboot/vmlinuz && \
    kvm-xfstests --kernel /tmp/vmlinuz \
                    --initrd /tmp/initrd.img smoke
