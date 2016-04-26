#!/bin/bash

BUCKET=@BUCKET@
GS_TAR=@GS_TAR@
BLD_INST=@BLD_INST@
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
	lighttpd \
	lvm2 \
	nano \
	perl \
	postfix \
	procps \
	psmisc \
	strace \
	time \
	xz-utils"

touch /run/gce-xfstests-bld

apt-get update
apt-get -y --with-new-pkgs upgrade
apt-get install -y debconf-utils
debconf-set-selections <<EOF
kexec-tools	kexec-tools/use_grub_config	boolean	true
kexec-tools	kexec-tools/load_kexec	boolean	true
postfix	postfix/destinations	string	xfstests.internal, localhost
postfix	postfix/mailname	string	xfstests.internal
postfix	postfix/main_mailer_type	select	Local only
EOF
apt-get install -y $PACKAGES
apt-get clean

sed -i.bak -e "/PermitRootLogin no/s/no/yes/" /etc/ssh/sshd_config

gsutil cp gs://$BUCKET/xfstests.tar.gz /run/xfstests.tar.gz
tar -C /root -xzf /run/xfstests.tar.gz
rm /run/xfstests.tar.gz

gsutil cp gs://$BUCKET/files.tar.gz /run/files.tar.gz
tar -C / -xzf /run/files.tar.gz
rm /run/files.tar.gz

for i in /results/runtests.log /var/log/syslog \
       /var/log/messages /var/log/kern.log
do
    ln -s "$i" /var/www
done

for i in diskstats meminfo lockdep lock_stat slabinfo vmstat
do
    ln /var/www/cgi-bin/print_proc "/var/www/cgi-bin/$i"
done
rm -rf /var/www/html

sed -e 's;/dev/;/dev/mapper/xt-;' < /root/test-config > /tmp/test-config
echo "export RUN_ON_GCE=yes" >> /tmp/test-config
mv /tmp/test-config /root/test-config
rm -f /root/*~
chown root:root /root

. /root/test-config

mkdir -p $PRI_TST_MNT $SM_SCR_MNT $SM_TST_MNT $LG_TST_MNT $LG_SCR_MNT /results
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
chmod 755 /root

systemctl enable kvm-xfstests.service
systemctl enable gce-finalize.service

if gsutil -m cp gs://$BUCKET/*.deb /run
then
    dpkg -i --ignore-depends=e2fsprogs /run/*.deb
    rm -f /run/*.deb
fi

gcloud components -q update

# Install logging agent
curl https://storage.googleapis.com/signals-agents/logging/google-fluentd-install.sh | bash
ZONE=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/zone" -H "Metadata-Flavor: Google")
ID=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/id" -H "Metadata-Flavor: Google")
logger -s "xfstests GCE appliance build completed (build instance id $ID)"

. /usr/local/lib/gce-funcs
rm -rf $GCE_STATE_DIR

fast=$(gce_attribute fast)

# This only works if with the very latest tune2fs, since the root
# file system is mounted here.  Make sure we the root file system
# has a unique UUID.
logger "Original root file system UUID $(blkid -s UUID -o value /dev/sda1)"
if /sbin/tune2fs -f -U random -L xfstests-root /dev/sda1
then
    NEW_UUID=$(blkid -s UUID -o value /dev/sda1)
    logger "Root file system UUID now $NEW_UUID"
    ed /etc/fstab <<EOF
/^UUID/s/UUID=[a-f0-9-]*/UUID=$NEW_UUID/
w
q
EOF
    /usr/sbin/update-grub
    /usr/sbin/update-initramfs -u -k all
fi

journalctl > /image-build.log
sync

if test "$fast" = "yes"
then
    fstrim /
    gcloud compute -q instances delete "$BLD_INST" --zone $(basename $ZONE) \
	   --keep-disks boot
else
    mount -t tmpfs -o size=10G tmpfs /mnt
    mkdir -p /mnt/tmp
    gcimagebundle -d /dev/sda -o /mnt/tmp/ -f ext3 --log_file=/tmp/bundle.log
    gsutil cp /mnt/tmp/*.image.tar.gz "$GS_TAR"
    gcloud compute -q instances delete "$BLD_INST" --zone $(basename $ZONE)
fi
