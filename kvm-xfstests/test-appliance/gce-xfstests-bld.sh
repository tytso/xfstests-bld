#!/bin/bash

# Close stdout and stderr
exec 1<&-
exec 2<&-
exec 1<>/image-build.log
exec 2>&1
set -vx

BUCKET=@BUCKET@
GS_TAR=@GS_TAR@
BLD_INST=@BLD_INST@
BACKPORTS="@BACKPORTS@"
BTRFS_PROGS="@BTRFS_PROGS@"
F2FS_TOOLS="@F2FS_TOOLS@"
DUPEREMOVE="@DUPEREMOVE@"

PACKAGES="bash-completion \
	bc \
	bison \
	blktrace \
	bsdmainutils \
	bsd-mailx \
	"$BTRFS_PROGS" \
	build-essential \
	bzip2 \
	cpio \
	cryptsetup \
	curl \
	dc \
	dbench \
	dbus \
	dmsetup \
	"$DUPEREMOVE" \
	dump \
	"e2fsprogs$BACKPORTS" \
	e3 \
	ed \
	"$F2FS_TOOLS" \
	file \
	flex \
	gawk \
	git	\
	jfsutils \
	jq \
	kexec-tools \
	keyutils \
	less \
	libcap2-bin \
	"libcomerr2$BACKPORTS" \
	libelf-dev \
	libsasl2-modules \
	"libss2$BACKPORTS" \
	liblzo2-2 \
	libkeyutils1 \
	libncurses-dev \
	libssl-dev \
	lighttpd \
	lvm2 \
	mtd-utils \
	multipath-tools \
	nano \
	nbd-client \
	nbd-server \
	nfs-common \
	nfs-kernel-server \
	nvme-cli \
	openssl \
	pciutils \
	perl \
	procps \
	psmisc \
	python-future \
	python-gdbm \
	python-pip \
	reiserfsprogs \
	strace \
	stress \
	thin-provisioning-tools \
	time \
	xz-utils"

PACKAGES_REMOVE="e2fsprogs-l10n"

if test -z "$MDS_PREFIX"
then
    declare -r MDS_PREFIX=http://metadata.google.internal/computeMetadata/v1
    declare -r MDS_TRIES=${MDS_TRIES:-100}
fi

function print_metadata_value() {
  local readonly tmpfile=$(mktemp)
  http_code=$(curl -f "${1}" -H "Metadata-Flavor: Google" -w "%{http_code}" \
    -s -o ${tmpfile} 2>/dev/null)
  local readonly return_code=$?
  # If the command completed successfully, print the metadata value to stdout.
  if [[ ${return_code} == 0 && ${http_code} == 200 ]]; then
    cat ${tmpfile}
  fi
  rm -f ${tmpfile}
  return ${return_code}
}

function print_metadata_value_if_exists() {
  local return_code=1
  local readonly url=$1
  print_metadata_value ${url}
  return_code=$?
  return ${return_code}
}

function get_metadata_value() {
  local readonly varname=$1
  # Print the instance metadata value.
  print_metadata_value_if_exists ${MDS_PREFIX}/instance/${varname}
  return_code=$?
  # If the instance doesn't have the value, try the project.
  if [[ ${return_code} != 0 ]]; then
    print_metadata_value_if_exists ${MDS_PREFIX}/project/${varname}
    return_code=$?
  fi
  return ${return_code}
}

function get_metadata_value_with_retries() {
  local return_code=1  # General error code.
  for ((count=0; count <= ${MDS_TRIES}; count++)); do
    get_metadata_value $1
    return_code=$?
    case $return_code in
      # No error.  We're done.
      0) return ${return_code};;
      # Failed to resolve host or connect to host.  Retry.
      6|7) sleep 0.3; continue;;
      # A genuine error.  Exit.
      *) return ${return_code};
    esac
  done
  # Exit with the last return code we got.
  return ${return_code}
}

function gce_attribute() {
	get_metadata_value_with_retries attributes/$1
}

touch /run/gce-xfstests-bld

cp -f /lib/systemd/system/serial-getty@.service \
	/etc/systemd/system/telnet-getty@.service
sed -i -e '/ExecStart/s/agetty/agetty -a root/' \
    -e '/ExecStart/s/-p/-p -f/' \
    -e 's/After=rc.local.service/After=network.target/' \
	/etc/systemd/system/telnet-getty@.service

systemctl enable telnet-getty@ttyS1.service
systemctl enable telnet-getty@ttyS2.service
systemctl enable telnet-getty@ttyS3.service
systemctl start telnet-getty@ttyS1.service
systemctl start telnet-getty@ttyS2.service
systemctl start telnet-getty@ttyS3.service

sed -i -e '/Conflicts=/iConditionPathExists=/sys/module/sunrpc' \
	/lib/systemd/system/run-rpc_pipefs.mount
sed -i -e '/Conflicts=/iConditionPathExists=/sys/module/nfsd' \
	/lib/systemd/system/proc-fs-nfsd.mount

apt-get update
apt-get install -y debconf-utils curl
debconf-set-selections <<EOF
kexec-tools	kexec-tools/use_grub_config	boolean	true
kexec-tools	kexec-tools/load_kexec		boolean	true
man-db		man-db/auto-update 		boolean false
keyboard-configuration	keyboard-configuration/variant	select	English (US)
grub-pc	grub-pc/install_devices	multiselect	/dev/sda
EOF
rm -f /var/lib/man-db/auto-update

export DEBIAN_FRONTEND=noninteractive

NEW_SUITE=$(gce_attribute suite)
OLD_SUITE=$(cat /etc/apt/sources.list | grep ^deb | grep -v updates | head -1 | awk '{print $3}')
if test -n "$NEW_SUITE" -a "$OLD_SUITE" != "$NEW_SUITE" ; then
    sed -e "s/$OLD_SUITE/$NEW_SUITE/g" < /etc/apt/sources.list > /etc/apt/sources.list.new
    mv /etc/apt/sources.list.new /etc/apt/sources.list
    apt-get update
    apt-get -y dist-upgrade
    apt-get -o Dpkg::Options::="--force-confnew" --force-yes -fuy dist-upgrade
    apt-get -fy autoremove
    logger -s "Update to $NEW_SUITE complete"
else
    apt-get update
    apt-get -y --with-new-pkgs upgrade
fi

if test "$NEW_SUITE" = "buster" ; then
    PACKAGES="$PACKAGES duperemove"
fi

apt-get install -y $PACKAGES
dpkg --remove $PACKAGES_REMOVE
apt-get -fuy autoremove
apt-get clean

PHORONIX=$(gce_attribute phoronix)
if test -z "${PHORONIX}" ; then
    fn=$(curl -s http://phoronix-test-suite.com/releases/repo/pts.debian/files/ | grep href | grep phoronix-test-suite | sed -e 's/^.*href="//' | sed -e 's/".*$//'  | sort -u  | tail -1)
    case "$fn" in
	phoronix-test-suite*all.deb) ;;
	*) fn="" ;;
    esac
else
    fn="phoronix-test-suite_${PHORONIX}_all.deb"
fi
if test -n "$fn" ; then
    curl -o /tmp/pts.deb "http://phoronix-test-suite.com/releases/repo/pts.debian/files/$fn"
    apt-get install -y php-cli php-xml unzip
    dpkg -i /tmp/pts.deb
    rm -f /tmp/pts.deb
    mkdir -p /var/lib/phoronix-test-suite
fi

sed -i.bak -e "/PermitRootLogin no/s/no/yes/" /etc/ssh/sshd_config

gsutil -m cp gs://$BUCKET/create-image/xfstests.tar.gz \
	gs://$BUCKET/create-image/files.tar.gz /run/
tar -C /root -xzf /run/xfstests.tar.gz
tar -C / -xzf /run/files.tar.gz
rm /run/xfstests.tar.gz /run/files.tar.gz

# this is to install some python packages into the image for
# the LTM web server.
pip install -r /usr/local/lib/gce-ltm/requirements.txt

for i in /results/runtests.log /var/log/syslog \
       /var/log/messages /var/log/kern.log
do
    ln -s "$i" /var/www
done

for i in diskstats meminfo lockdep lock_stat slabinfo vmstat
do
    ln /usr/lib/cgi-bin/print_proc "/usr/lib/cgi-bin/$i"
done
rm -rf /var/www/html /var/www/cgi-bin
ln -s /usr/lib/cgi-bin /var/www/cgi-bin
chown www-data:www-data -R /usr/local/lib/gce-ltm
chown www-data:www-data -R /var/www

lighttpd-enable-mod ssi
lighttpd-enable-mod ssl
lighttpd-enable-mod cgi
ed /etc/lighttpd/lighttpd.conf <<EOF
/^server.document-root/s/^/#/p
/^index-file.names/s/^/#/p
/^include_shell.*create-mime/s/^/#/p
w
q
EOF
cp /etc/lighttpd/lighttpd.conf /etc/lighttpd/lighttpd-orig.conf
cat /etc/lighttpd/ltm.conf >> /etc/lighttpd/lighttpd.conf

sed -e 's;/dev/;/dev/mapper/xt-;' < /root/test-config > /tmp/test-config
echo "export RUN_ON_GCE=yes" >> /tmp/test-config
mv /tmp/test-config /root/test-config
rm -f /root/*~
chown root:root /root

. /root/test-config

mkdir -p $PRI_TST_MNT $SM_SCR_MNT $SM_TST_MNT $LG_TST_MNT $LG_SCR_MNT \
      /results /test /scratch /mnt/test /mnt/scratch
touch /results/runtests.log

cat >> /etc/fstab <<EOF
LABEL=results	/results ext4	noauto 0 2
EOF

ed /etc/lvm/lvm.conf <<EOF
/issue_discards = /s/0/1/
w
q
EOF

echo "fsgqa:x:31415:31415:fsgqa user:/home/fsgqa:/bin/bash" >> /etc/passwd
echo "fsgqa:!::0:99999:7:::" >> /etc/shadow
echo "fsgqa:x:31415:" >> /etc/group
echo "fsgqa:!::" >> /etc/gshadow
mkdir -p /home/fsgqa
chown 31415:31415 /home/fsgqa
echo "123456-fsgqa:x:31416:31416:numberic fsgqa user:/home/123456-fsgqa:/bin/bash" >> /etc/passwd
echo "123456-fsgqa:!::0:99999:7:::" >> /etc/shadow
echo "123456-fsgqa:x:31416:" >> /etc/group
echo "123456-fsgqa:!::" >> /etc/gshadow
mkdir /home/123456-fsgqa
chown 31416:31416 /home/123456-fsgqa
chmod 755 /root

cp -f /lib/systemd/system/serial-getty@.service \
	/etc/systemd/system/telnet-getty@.service
sed -i -e '/ExecStart/s/agetty/agetty -a root/' \
    -e '/ExecStart/s/-p/-p -f/' \
    -e 's/After=rc.local.service/After=kvm-xfstests.service/' \
	/lib/systemd/system/serial-getty@.service
sed -i -e '/ExecStart/s/agetty/agetty -a root/' \
    -e '/ExecStart/s/-p/-p -f/' \
    -e 's/After=rc.local.service/After=network.target/' \
	/etc/systemd/system/telnet-getty@.service

systemctl enable kvm-xfstests.service
systemctl enable gce-fetch-gs-files.service
systemctl enable gce-finalize-wait.service
systemctl enable gce-finalize.timer
systemctl enable gen-ssh-keys.service
systemctl enable telnet-getty@ttyS1.service
systemctl enable telnet-getty@ttyS2.service
systemctl enable telnet-getty@ttyS3.service
systemctl stop multipathd
systemctl disable multipathd

if test -f /etc/default/nfs-kernel-server ; then
    ed /etc/default/nfs-kernel-server <<EOF
/RPCNFSDCOUNT/c
RPCNFSDCOUNT="8 --nfs-version 2"
.
w
q
EOF
fi

if gsutil -m cp gs://$BUCKET/debs/*_amd64.deb /run
then
    dpkg -i --ignore-depends=e2fsprogs --auto-deconfigure /run/*.deb
    rm -f /run/*.deb
fi
chmod +rx /usr/local/lib/gce-ltm/gce-xfs-ltm.fcgi
chmod +rx /usr/local/lib/gce-ltm/app.py

gcloud components -q update

ZONE=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/zone" -H "Metadata-Flavor: Google")
ID=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/id" -H "Metadata-Flavor: Google")
logger -s "xfstests GCE appliance build completed (build instance id $ID)"

. /usr/local/lib/gce-funcs
rm -rf $GCE_STATE_DIR

# Set label
/sbin/tune2fs -L xfstests-root /dev/sda1

find /var/cache/man /var/cache/apt /var/lib/apt/lists -type f -print | xargs rm
rm -f /etc/ssh/ssh_host_key* /etc/ssh/ssh_host_*_key*
rm -rf /root/.cache/*
sync
fstrim /
gcloud compute -q instances delete "$BLD_INST" --zone $(basename $ZONE) \
	--keep-disks boot
