ARCH_LIST=(amd64 i386 arm64)
MIRROR=http://snapshot.debian.org/archive/debian/20210411T144547Z/

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=4072b9d338304fa610d768ceff53f8767440e4f9 # 2021-04-11 release

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=a97792a88b93ef882194342a7fe40c77c6c83146 # v5.11.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=267b164c372d57145880f365bab8d8a52bf8baa7 # fio-3.26

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=25f16b1de313ce0d411f754572f94f051bfbe3c8

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=bab43553a904660266fdcd8fb974c7bdd96b3f58

FSVERITY_GIT=https://git.kernel.org/pub/scm/linux/kernel/git/ebiggers/fsverity-utils.git/
FSVERITY_COMMIT=cf8fa5e5a7ac5b3b2dbfcc87e5dbd5f984c2d83a # v1.3-2-gcf8fa5e

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=00a0e66a14d3663edd9d37c8a01db6d182c88bdd # v1.3.2

BLKTESTS_GIT=https://github.com/tytso/blktests.git
BLKTESTS_COMMIT=4bc88ef88a6aaa84b5d45caea5d3f8f75f86447f

NVME_CLI_GIT=https://github.com/linux-nvme/nvme-cli
NVME_CLI_COMMIT=f0e9569df9289d6ee55ba2c23615cc7c73a9b088 # v1.13

UTIL_LINUX_GIT=https://git.kernel.org/pub/scm/utils/util-linux/util-linux.git
UTIL_LINUX_COMMIT=b897734b57ea06643fa916f15270f21ea2f14431 # v2.36.2
UTIL_LINUX_LIBS_ONLY=yes

#EXEC_LDFLAGS=-static
#EXEC_LLDFLAGS=-all-static
export PATH=$HOME/bin-ccache:/bin:/usr/bin
export CCACHE_DIR=/var/cache/ccache
export CCACHE_COMPRESS=t

BUILD_ENV="schroot -c $CHROOT --"
SUDO_ENV="schroot -c $CHROOT -u root --"
OUT_TAR="both"
gen_image_args="--networking"
