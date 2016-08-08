#!/bin/bash

BUCKET="@BUCKET@"
GS_TAR="@GS_TAR@"
GCE_ZONE="@GCE_ZONE@"
GCE_IMAGE_PROJECT="@GCE_IMAGE_PROJECT@"
GCE_PROJECT="@GCE_PROJECT@"
IMAGE_FLAG="@IMAGE_FLAG@"
ROOT_FS="@ROOT_FS@"
IMG_DISK="@IMG_DISK@"
EXP_INST="@EXP_INST@"

apt-get install dstat pigz

gcloud compute --project "$GCE_PROJECT" -q disks create "$IMG_DISK" \
       --image-project "${GCE_IMAGE_PROJECT:-xfstests-cloud}" \
       "$IMAGE_FLAG" "$ROOT_FS" --type pd-standard --zone "$GCE_ZONE" \
       --size 10GB

gcloud compute --project "$GCE_PROJECT" -q instances attach-disk "$EXP_INST" \
	   --disk "$IMG_DISK" \
	   --device-name image-disk --zone "$GCE_ZONE"

gcloud compute --project "$GCE_PROJECT" -q instances set-disk-auto-delete \
       "$EXP_INST" --auto-delete --disk "$IMG_DISK" --zone "$GCE_ZONE" &

mkdir /mnt/tmp /mnt/image-disk
mount -t tmpfs -o size=20g tmpfs /mnt/tmp

#
# Since the gce-xfstests image was derived from the Debian image, it
# has the same UUID as the stock Debian image.  This has the potential
# for a lot of confusion, which is why we create the image-disk and
# attach it to the instance above, instead of in the gce-export-image
# script --- if we do that there will be two disks with the same UUID
# and debian may end up using the wrong volume as the mounted root
# disk.  So before we extract out the image, change the UUID to a new,
# random one, to avoid future potential hard-to-debug problems.
#
image_dev=/dev/disk/by-id/google-image-disk-part1
old_uuid=$(tune2fs -l "$image_dev" | grep "Filesystem UUID:" | awk '{print $3}')
new_uuid=$(uuidgen)
e2fsck -fy -E discard "$image_dev"

# this doesn't work with tune2fs 1.42.12, which is used by jessie by default
# tune2fs -U "$new_uuid" /dev/disk/by-id/google-image-disk-part1
#
# So we do this instead --- which might not work well in next verison of
# debian if we turn on metadata_csum by default.  So when we switch to Debian
# Stretch (Debian 9.0), we will need to  switch back to the tune2fs command
debugfs -R "ssv uuid $new_uuid" -w "$image_dev"

e2fsck -fy "$image_dev"
mount "$image_dev" /mnt/image-disk
sed -ie "s/$old_uuid/$new_uuid/" /mnt/image-disk/etc/fstab
sed -ie "s/$old_uuid/$new_uuid/" /mnt/image-disk/boot/grub/grub.cfg
umount /mnt/image-disk

dd if=/dev/disk/by-id/google-image-disk of=/mnt/tmp/disk.raw conv=sparse bs=4096
cd /mnt/tmp
tar cvf - disk.raw | pigz > myimage.tar.gz

gsutil cp /mnt/tmp/myimage.tar.gz $GS_TAR

gcloud compute -q instances delete "$EXP_INST" --zone "$GCE_ZONE"
