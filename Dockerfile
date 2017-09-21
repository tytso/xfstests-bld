# This Dockerfile creates an debian image with xfstests-bld build environment
#
# This is more for the sake of showing how to use a Dockerfile to
# build the xfstests.tar.gz file more than anything else.  (The resulting
# docker image is almost twice as big as it would be if we didn't try
# building it inside Docker.)
#
# VERSION 0.1
FROM debian:stretch

# Install dependencies
RUN apt-get update && \
    apt-get install -y \
	    autoconf \
	    automake \
	    bc \
	    build-essential \
	    curl \
	    gettext \
	    git \
	    libtool \
	    libtool-bin \
	    pkg-config \
	    pigz \
	    uuid-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* \
       /usr/share/doc /usr/share/doc-base \
       /usr/share/man /usr/share/locale /usr/share/zoneinfo

MAINTAINER Theodore Y. Ts'o tytso@mit.edu

COPY . /devel/xfstests-bld

RUN cd /devel/xfstests-bld && \
    cp config config.custom && \
    echo "XFSTESTS_GIT=https://github.com/tytso/xfstests" >> config.custom && \
    make && \
    make tarball && \
    tar -C /root -xf xfstests.tar.gz && \
    cd kvm-xfstests/test-appliance && \
    cp docker-entrypoint /entrypoint && \
    rsync --exclude-from docker-exclude-files -avH files/* / && \
    chown -R root:root /root && \
    useradd -u 31415 -s /bin/bash -m fsgqa && \
    cd /devel && \
    rm -rf /devel/xfstests-bld && \
    mkdir -p /results && \
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

ENTRYPOINT ["/entrypoint"]
CMD ["-g","quick"]
