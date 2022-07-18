ARCH_LIST=(amd64 i386 arm64)
MIRROR=https://snapshot.debian.org/archive/debian/20220108T035927Z/
DISTRO=bullseye

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=f0a05db9bc68455b00c1363369211d39ab41df7a # 2022-01-09 release

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=b42033308360655616fc9bd77678c46bf518b7c8 # v5.13.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=9b46661c289d01dbfe5182189a7abea9ce2f9e04 # fio-3.29

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=d2256ac2d44b0a5be9c0b49ce4ce8e5f6821ce2a

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=bab43553a904660266fdcd8fb974c7bdd96b3f58

FSVERITY_GIT=https://git.kernel.org/pub/scm/linux/kernel/git/ebiggers/fsverity-utils.git/
FSVERITY_COMMIT=ddc6bc9daeb79db932aa12edb85c7c2f4647472a # v1.4-4-gddc6bc9

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=00a0e66a14d3663edd9d37c8a01db6d182c88bdd # v1.3.2

BLKTESTS_GIT=https://github.com/tytso/blktests.git
BLKTESTS_COMMIT=af97b557d52aebd94c0d9d1d2f1bbf759bbc75df

NVME_CLI_GIT=https://github.com/linux-nvme/nvme-cli
NVME_CLI_COMMIT=deee9cae1ac94760deebd71f8e5449061338666c # v1.16

UTIL_LINUX_GIT=https://git.kernel.org/pub/scm/utils/util-linux/util-linux.git
UTIL_LINUX_COMMIT=f59c8fd38dfee24b93ed54a6984f879499c34ec7 # v2.37.2
UTIL_LINUX_LIBS_ONLY=yes

#EXEC_LDFLAGS=-static
#EXEC_LLDFLAGS=-all-static
export PATH=$HOME/bin-ccache:/bin:/usr/bin
export CCACHE_DIR=/var/cache/ccache
export CCACHE_COMPRESS=t

#BEGIN BUILD-CONFIG

BUILD_ENV="schroot -c $CHROOT --"
SUDO_ENV="schroot -c $CHROOT -u root --"
OUT_TAR="both"
gen_image_args="--networking"
