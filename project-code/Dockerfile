# This Dockerfile creates an debian image with xfstests-bld build environment
#
# This Dockerfile file is useful for building the xfstests.tar.gz file
# in a Docker environment, for continuous build testing.  It can also
# be useful for testing whatever the file system environemnt is
# provided in the Docker environment, without requiring any special
# privileges.
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
	    libkeyutils-dev \
	    libtool \
	    libtool-bin \
	    pkg-config \
	    pigz \
	    uuid-dev \
	    libssl-dev \
	    autopoint  \
	    procps \
	    python \
	    python-future && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* \
       /usr/share/doc /usr/share/doc-base \
       /usr/share/man /usr/share/locale /usr/share/zoneinfo

MAINTAINER Theodore Y. Ts'o tytso@mit.edu

COPY . /devel/xfstests-bld

RUN cd /devel/xfstests-bld && \
    cp config.docker config.custom && \
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
	    autopoint \
	    build-essential \
	    gettext \
	    git \
	    libkeyutils-dev \
	    libssl-dev \
	    libtool \
	    libtool-bin \
	    pkg-config \
	    pigz \
	    uuid-dev && \
    apt-get autoremove -y

ENTRYPOINT ["/entrypoint"]
CMD ["-g","quick"]
