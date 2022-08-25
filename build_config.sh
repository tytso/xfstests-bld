ARCH_LIST=(amd64 i386 arm64)
MIRROR=https://snapshot.debian.org/archive/debian/20220823T031903Z/
DISTRO=bullseye

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=289f50f8b30a5372ab66d293df5563efc7cf040b # 2022-08-22 release

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=5652dc4fbb6c9e7a4911ddcc4c3e373a4d014a6f # v5.19.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=6cafe8445fd1e04e5f7d67bbc73029a538d1b253 # fio-3.31

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=d2256ac2d44b0a5be9c0b49ce4ce8e5f6821ce2a

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=bab43553a904660266fdcd8fb974c7bdd96b3f58

FSVERITY_GIT=https://git.kernel.org/pub/scm/linux/kernel/git/ebiggers/fsverity-utils.git/
FSVERITY_COMMIT=20e87c13075a8e5660a8d69fd6c93d4f7c5f01a5 # v1.5

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=00a0e66a14d3663edd9d37c8a01db6d182c88bdd # v1.3.2

BLKTESTS_GIT=https://github.com/tytso/blktests.git
BLKTESTS_COMMIT=4e07b0c360db5e74d3cbe5b3c6c8b86199017fbf

NVME_CLI_GIT=https://github.com/linux-nvme/nvme-cli
NVME_CLI_COMMIT=deee9cae1ac94760deebd71f8e5449061338666c # v1.16

UTIL_LINUX_GIT=https://git.kernel.org/pub/scm/utils/util-linux/util-linux.git
UTIL_LINUX_COMMIT=54a4d5c3ec33f2f743309ec883b9854818a25e31 # v2.38.1
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
