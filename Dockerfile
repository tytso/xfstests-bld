# This Dockerfile creates an debian image with xfstests-bld build environment
#
# This is more for the sake of showing how to use a Dockerfile to
# build the xfstests.tar.gz file more than anything else.  (The resulting
# docker image is almost twice as big as it would be if we didn't try
# building it inside Docker.)
#
# VERSION 0.1
FROM debian

MAINTAINER Theodore Y. Ts'o tytso@mit.edu

COPY . /devel/xfstests-bld

# Install deps.
RUN apt-get update && \
    apt-get install -y \
	    autoconf \
	    automake \
	    build-essential \
	    curl \
	    gettext \
	    git \
	    libtool \
	    libtool-bin \
	    pkg-config \
	    pigz \
	    qemu-kvm \
	    qemu-utils \
	    uuid-dev \
	    net-tools \
	    iptables && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* \
       /usr/share/doc /usr/share/doc-base \
       /usr/share/man /usr/share/locale /usr/share/zoneinfo && \
    cd /devel/xfstests-bld && \
    make && \
    make tarball && \
    make -C kvm-xfstests prefix=/usr/local \
        PREBUILT_URL=https://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests/root_fs.img.x86_64 install && \
	cp xfstests.tar.gz /usr/local/lib && \
    cd /devel && \
    rm -rf /devel/xfstests-bld && \
    apt-get purge -y \
	    autoconf \
	    automake \
	    build-essential \
	    gettext \
	    git \
	    libtool \
	    libtool-bin \
	    pkg-config \
	    pigz \
	    uuid-dev && \
    apt-get autoremove -y

# This is build enviroment so there is no sane default command here,
# this command simply demonstrate that the enviroment is sane
CMD curl -o /tmp/initrd.img https://dl.fedoraproject.org/pub/fedora/linux/releases/24/Server/x86_64/os/images/pxeboot/initrd.img && \
    curl -o /tmp/vmlinuz https://dl.fedoraproject.org/pub/fedora/linux/releases/24/Server/x86_64/os/images/pxeboot/vmlinuz && \
    kvm-xfstests --kernel /tmp/vmlinuz \
       		    --initrd /tmp/initrd.img \
		    --update-files --update-xfstests-tar smoke
