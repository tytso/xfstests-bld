# This Dockerfile creates a kvm-xfstests runtime environment
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

ARG IMAGE_URL=https://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests/root_fs.img.i386

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
        curl -o root_fs.img $IMAGE_URL ; \
    fi

ENV SAMPLE_KERNEL_URL=https://dl.fedoraproject.org/pub/fedora/linux/releases/26/Server/x86_64/os/images/pxeboot

# The default command serves merely as a demo and can be used as a
# sanity check that docker image was built correctly.
CMD curl -o /tmp/initrd.img $SAMPLE_KERNEL_URL/initrd.img && \
    curl -o /tmp/vmlinuz $SAMPLE_KERNEL_URL/vmlinuz && \
    kvm-xfstests --kernel /tmp/vmlinuz \
                    --initrd /tmp/initrd.img smoke
