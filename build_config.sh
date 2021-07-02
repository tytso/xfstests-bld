ARCH_LIST=(amd64 i386 arm64)
MIRROR=http://snapshot.debian.org/archive/debian/20210411T144547Z/

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=6aa18fb84823a83040719b6490c93910d1e22196 # 2021-07-02 release

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=3e384caad5663aed3071a1dff3da85b9ab5129dc # v5.12.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=0313e938c9c8bb37d71dade239f1f5326677b079 # fio-3.27

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=25f16b1de313ce0d411f754572f94f051bfbe3c8

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=bab43553a904660266fdcd8fb974c7bdd96b3f58

FSVERITY_GIT=https://git.kernel.org/pub/scm/linux/kernel/git/ebiggers/fsverity-utils.git/
FSVERITY_COMMIT=9e082897d61a2449657651aa5a0931aca31428fd # v1.4

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=00a0e66a14d3663edd9d37c8a01db6d182c88bdd # v1.3.2

BLKTESTS_GIT=https://github.com/tytso/blktests.git
BLKTESTS_COMMIT=62ec9d600e2a2cfe5589fc4343eede02b90a2555

NVME_CLI_GIT=https://github.com/linux-nvme/nvme-cli
NVME_CLI_COMMIT=f729e93e2c08ed87ea0007238c729dbf911cd433 # v1.14-61-gf729e93

UTIL_LINUX_GIT=https://git.kernel.org/pub/scm/utils/util-linux/util-linux.git
UTIL_LINUX_COMMIT=50736e4998fde0fff9b7876476137a21b85bd5a6 # v2.37
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
