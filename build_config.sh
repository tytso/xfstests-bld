ARCH_LIST=(amd64 i386 arm64 armhf)
MIRROR=https://snapshot.debian.org/archive/debian/20190118T220436Z/

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=476f61a48d6499949096436065f9fdd8e9ab7c37

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=413a0f0d91a15b9eab0d8ea7a4ed773243f14f88 # v4.20.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=9f50b4106bd1d6fa1c325900d1fb286832ccc5e8 # fio-3.2

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=59b280ebe22eceaf4250cb3b776674619a4d4ece

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=bab43553a904660266fdcd8fb974c7bdd96b3f58

FSVERITY_GIT=https://git.kernel.org/pub/scm/linux/kernel/git/ebiggers/fsverity-utils.git/
FSVERITY_COMMIT=bdebc45b4527d64109723ad5753fa514bac47c9f

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=0267fa16990fd0ddcc89984a8e55b27d43e80167

BLKTESTS_GIT=https://github.com/tytso/blktests.git
BLKTESTS_COMMIT=baccddc0063f4e5706a73a4f17cdf83cc15fb10a

NVME_CLI_GIT=https://github.com/linux-nvme/nvme-cli
NVME_CLI_COMMIT=669d75939802b12598f66de95c9d6454c3ad6fa3

#EXEC_LDFLAGS=-static
#EXEC_LLDFLAGS=-all-static
export PATH=$HOME/bin-ccache:/bin:/usr/bin
export CCACHE_DIR=/var/cache/ccache
export CCACHE_COMPRESS=t

BUILD_ENV="schroot -c $CHROOT --"
SUDO_ENV="schroot -c $CHROOT -u root --"
gen_image_args="--networking"
