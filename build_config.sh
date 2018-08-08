ARCH_LIST=(amd64 i386 arm64 armhf)
MIRROR=http://snapshot.debian.org/archive/debian/20180529T032410Z/

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=34977a44c0d813e603c64567cc9d8b1c8ef32edd

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=v4.16.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=fio-3.2

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=59b280ebe22eceaf4250cb3b776674619a4d4ece

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=2f93b54f26aa40233a0a584ce8714e55c8dd159a

FSVERITY_GIT=git://git.kernel.org/pub/scm/linux/kernel/git/mhalcrow/fsverity.git
FSVERITY_COMMIT=2a7dbea90885dbd1dadc3d4a2873008ae618614e

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=5fa7d35de50c65ac58911ca4f7f0bb8c076d7ecf

#EXEC_LDFLAGS=-static
#EXEC_LLDFLAGS=-all-static
export PATH=$HOME/bin-ccache:/bin:/usr/bin
export CCACHE_DIR=/var/cache/ccache
export CCACHE_COMPRESS=t

BUILD_ENV="schroot -c $CHROOT --"
SUDO_ENV="schroot -c $CHROOT -u root --"
