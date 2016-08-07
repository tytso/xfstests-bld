#!/bin/bash

BUCKET=@BUCKET@
GS_TAR=@GS_TAR@
EXP_INST=@EXP_INST@

mkdir /mnt/tmp /mnt/image-disk
mkfs.ext4 -F /dev/disk/by-id/google-temporary-disk
mount -o discard,defaults /dev/disk/by-id/google-temporary-disk /mnt/tmp

# mount /dev/disk/by-id/google-image-disk /mnt/image-disk
# ...
# umount /mnt/image-disk

dd if=/dev/disk/by-id/google-image-disk of=/mnt/tmp/disk.raw bs=4096

cd /mnt/tmp
tar czvf myimage.tar.gz disk.raw

gsutil cp /mnt/tmp/myimage.tar.gz $GS_TAR

ZONE=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/zone" -H "Metadata-Flavor: Google")
gcloud compute -q instances delete "$EXP_INST" --zone $(basename $ZONE)
