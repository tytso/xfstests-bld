#!/bin/bash

BUCKET=@BUCKET@
PACKAGES="bash-completion \
	bc \
	bsdmainutils \
	bsd-mailx \
	bzip2 \
	cpio \
	dc \
	dbench \
	dbus \
	dmsetup \
	dump \
	e3 \
	ed \
	file \
	gawk \
	kexec-tools \
	keyutils \
	less \
	libsasl2-modules \
	libssl1.0.0 \
	libgdbm3 \
	lvm2 \
	nano \
	perl \
	postfix \
	procps \
	psmisc \
	strace \
	time \
	xz-utils"

apt-get install -y debconf-utils
debconf-set-selections <<EOF
kexec-tools	kexec-tools/use_grub_config	boolean	true
kexec-tools	kexec-tools/load_kexec	boolean	true
postfix	postfix/destinations	string	xfstests.internal, localhost
postfix	postfix/mailname	string	xfstests.internal
postfix	postfix/main_mailer_type	select	Local only
EOF
apt-get install -y $PACKAGES
gsutil cp gs://$BUCKET/xfstests.tar.gz /tmp/xfstests.tar.gz
cd /root
tar xfz /tmp/xfstests.tar.gz
rm /tmp/xfstests.tar.gz
sed -e 's;/dev/;/dev/mapper/xt-;' < /root/test-config > /tmp/test-config
echo "export RUN_ON_GCE=yes" >> /tmp/test-config
echo "export GS_BUCKET=$BUCKET" >> /tmp/test-config
mv /tmp/test-config /root/test-config
rm -f /root/*~
chown root:root /root

. /root/test-config

mkdir -p $PRI_TST_MNT $SM_SCR_MNT $SM_TST_MNT $LG_TST_MNT $LG_SCR_MNT /results

cat >> /etc/fstab <<EOF
/dev/sdb	/results ext4	noauto 0 2
EOF

echo "fsgqa:x:31415:31415:fsgqa user:/home/fsgqa:/bin/bash" >> /etc/passwd
echo "fsgqa:!::0:99999:7:::" >> /etc/shadow
echo "fsgqa:x:31415:" >> /etc/group
echo "fsgqa:!::" >> /etc/gshadow
mkdir -p /home/fsgqa
chown 31415:31415 /home/fsgqa
chmod 755 /root

mkdir -p /usr/local/sbin /usr/local/lib
mv /root/sbin/* /usr/local/sbin
mv /root/lib/gce-postfix-main.cf /etc/postfix/main.cf
mv /root/lib/* /usr/local/lib
rmdir /root/sbin /root/lib

mv /root/*.service /etc/systemd/system
systemctl enable kvm-xfstests.service

if gsutil -m cp gs://$BUCKET/*.deb /tmp
then
    dpkg -i --ignore-depends=e2fsprogs /tmp/*.deb
    rm -f /tmp/*.deb
fi

gcloud components -q update

# Install logging agent
curl https://storage.googleapis.com/signals-agents/logging/google-fluentd-install.sh | bash
ZONE=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/zone" -H "Metadata-Flavor: Google")
ID=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/id" -H "Metadata-Flavor: Google")
logger -s "xfstests GCE appliance build completed (build instance id $ID)"
journalctl > /image-build.log
fstrim /
mount -t tmpfs -o size=10G tmpfs /mnt
mkdir -p /mnt/tmp
gcimagebundle -d /dev/sda -o /mnt/tmp/ --log_file=/tmp/bundle.log
gsutil cp /mnt/tmp/*.image.tar.gz gs://$BUCKET/gce-xfstests.image.tar.gz
gcloud compute -q instances delete xfstests-bld --zone $(basename $ZONE)
