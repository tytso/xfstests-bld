ARCH_LIST=(amd64 i386 arm64)
MIRROR=http://snapshot.debian.org/archive/debian/20190826T092742Z/

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=dce1dac57c50ec6442e6353f50153f93e4b5e930

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=1609c11ae071e7c7b6309bf94f291faf1a2006b3 # v5.3.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=01bf5128d0581e267383f280c6a1dcd26517240f # fio-3.15

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=9a001cc6eb211758015d85cecc0464c94c82bbb5

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=bab43553a904660266fdcd8fb974c7bdd96b3f58

FSVERITY_GIT=https://git.kernel.org/pub/scm/linux/kernel/git/ebiggers/fsverity-utils.git/
FSVERITY_COMMIT=6585eb4968a0f3f0811bd8707ff5b04c78cf1c5e

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=515c99856ef52bbf680e6dd6c338acfb8d088614

BLKTESTS_GIT=https://github.com/tytso/blktests.git
BLKTESTS_COMMIT=e86852d221f38536fbb1ee595f2276d8a13d4be3

NVME_CLI_GIT=https://github.com/linux-nvme/nvme-cli
NVME_CLI_COMMIT=b7eb621f1a998f9b7a58501fdf6e6773ddc937ff

#EXEC_LDFLAGS=-static
#EXEC_LLDFLAGS=-all-static
export PATH=$HOME/bin-ccache:/bin:/usr/bin
export CCACHE_DIR=/var/cache/ccache
export CCACHE_COMPRESS=t

BUILD_ENV="schroot -c $CHROOT --"
SUDO_ENV="schroot -c $CHROOT -u root --"
gen_image_args="--networking"
