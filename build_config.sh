ARCH_LIST=(amd64 i386 arm64 armhf)
MIRROR=http://snapshot.debian.org/archive/debian/20180418T161205Z/

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=5cc2363625b752fda09c09903ffa8842af327d40

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=v4.16.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=fio-3.2

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=59b280e

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=d5a5d045176c3

FSVERITY_GIT=git://git.kernel.org/pub/scm/linux/kernel/git/mhalcrow/fsverity.git
FSVERITY_COMMIT=2a7dbea90885

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=v1.1

#EXEC_LDFLAGS=-static
#EXEC_LLDFLAGS=-all-static
export PATH=$HOME/bin-ccache:/bin:/usr/bin
export CCACHE_DIR=/var/cache/ccache
export CCACHE_COMPRESS=t

BUILD_ENV="schroot -c $CHROOT --"
SUDO_ENV="schroot -c $CHROOT -u root --"
