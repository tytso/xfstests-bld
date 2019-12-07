#!/bin/bash

DIR=/root
BUILDS="$DIR/builds"
KERNEL_DIR=''
GIT_REPO=$1
COMMIT=$2
KCONFIG=$3

if test -z $GIT_REPO
then GIT_REPO=https://github.com/torvalds/linux
fi

if test -z $COMMIT
then COMMIT=master
fi

if test -z $KERNEL_DIR
then KERNEL_DIR=${GIT_REPO##*/}
fi

if test -z $KCONFIG
then KCONFIG="$DIR/kernel-configs/x86_64-config-4.19"
fi

if test -z $GS_BUCKET
then GS_BUCKET=ec528-xfstests
fi

cd $BUILDS
git clone --reference $DIR/.gitcaches/linux.reference $GIT_REPO
cp "$KCONFIG" "$KERNEL_DIR/.config"
cd $KERNEL_DIR
git checkout $COMMIT
make olddefconfig
make -j$(nproc) > build.log

# copy image to gcs bucket
gsutil cp "$BUILDS/$KERNEL_DIR/arch/x86/boot/bzImage" "gs://$GS_BUCKET/bzImage"
