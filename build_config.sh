ARCH_LIST=(amd64 i386 arm64 armhf)
MIRROR=https://snapshot.debian.org/archive/debian/20190118T220436Z/

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=4ad32d1950b292c139c8479e4f0bace6fac818f4

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=v4.19.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=fio-3.2

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=59b280ebe22eceaf4250cb3b776674619a4d4ece

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=2f93b54f26aa40233a0a584ce8714e55c8dd159a

FSVERITY_GIT=https://git.kernel.org/pub/scm/linux/kernel/git/ebiggers/fsverity-utils.git/
FSVERITY_COMMIT=bdebc45b4527d64109723ad5753fa514bac47c9f

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=0267fa16990fd0ddcc89984a8e55b27d43e80167

BLKTESTS_GIT=https://github.com/osandov/blktests.git
BLKTESTS_COMMIT=4cc46670a681cb2f039f9c92994eaa7455de841f

NVME_CLI_GIT=https://github.com/linux-nvme/nvme-cli
NVME_CLI_COMMIT=eb58f548342f477d02da555cb939b3e6a5609269

#EXEC_LDFLAGS=-static
#EXEC_LLDFLAGS=-all-static
export PATH=$HOME/bin-ccache:/bin:/usr/bin
export CCACHE_DIR=/var/cache/ccache
export CCACHE_COMPRESS=t

BUILD_ENV="schroot -c $CHROOT --"
SUDO_ENV="schroot -c $CHROOT -u root --"
gen_image_args="--networking"
