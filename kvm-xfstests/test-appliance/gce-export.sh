#!/bin/bash

if test -z "$DO_SCRIPT" ; then
   export DO_SCRIPT=yes
   exec script -f -c "$0 $*" /root/gce-export.log
fi

date

BUCKET="@BUCKET@"
GS_TAR="@GS_TAR@"
GCE_ZONE="@GCE_ZONE@"
GCE_IMAGE_PROJECT="@GCE_IMAGE_PROJECT@"
GCE_PROJECT="@GCE_PROJECT@"
IMAGE_FLAG="@IMAGE_FLAG@"
ROOT_FS="@ROOT_FS@"
SKIP_UUID="@SKIP_UUID@"
IMG_DISK="@IMG_DISK@"
EXP_INST="@EXP_INST@"

apt-get install dstat pigz

if test -n "$ROOT_FS"; then
    gcloud compute --project "$GCE_PROJECT" -q disks create "$IMG_DISK" \
	--image-project "${GCE_IMAGE_PROJECT:-xfstests-cloud}" \
	"$IMAGE_FLAG" "$ROOT_FS" --type pd-standard --zone "$GCE_ZONE"
else
    GCE_PROJECT="$GCE_IMAGE_PROJECT"
fi

gcloud compute --project "$GCE_PROJECT" -q instances attach-disk "$EXP_INST" \
	   --disk "$IMG_DISK" \
	   --device-name image-disk --zone "$GCE_ZONE"

if test -n "$ROOT_FS"; then
    gcloud compute --project "$GCE_PROJECT" -q instances set-disk-auto-delete \
	"$EXP_INST" --auto-delete --disk "$IMG_DISK" --zone "$GCE_ZONE" &
fi

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
if test -z "$SKIP_UUID" ; then
    image_dev=/dev/disk/by-id/google-image-disk-part1
    old_uuid=$(tune2fs -l "$image_dev" | grep "Filesystem UUID:" | awk '{print $3}')
    new_uuid=$(uuidgen)
    e2fsck -fy "$image_dev"

    echo "Changing UUID from $old_uuid to $new_uuid"
    tune2fs -U "$new_uuid" "$image_dev"
    e2fsck -fy -E discard "$image_dev"

    mount "$image_dev" /mnt/image-disk
    sed -ie "s/$old_uuid/$new_uuid/" /mnt/image-disk/etc/fstab
    sed -ie "s/$old_uuid/$new_uuid/" /mnt/image-disk/boot/grub/grub.cfg
    umount /mnt/image-disk
fi

dd if=/dev/disk/by-id/google-image-disk of=/mnt/tmp/disk.raw conv=sparse bs=4096
cd /mnt/tmp
tar cvf - disk.raw | pigz > myimage.tar.gz

date
gsutil -q cp /mnt/tmp/myimage.tar.gz $GS_TAR
gsutil -q cp /root/gce-export.log "gs://$BUCKET/gce-export.log"

gcloud compute -q instances delete "$EXP_INST" --zone "$GCE_ZONE"
