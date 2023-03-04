ARCH_LIST=(amd64 i386 arm64)
MIRROR=https://snapshot.debian.org/archive/debian/20220823T031903Z/
DISTRO=bullseye

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=821ef4889cb6a568a237a52ca0eee0332188d049 # release-2023-03-03-821ef4889

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=fdf0366959f1d04f2aea93a3fac24c49b9d5e55f # v6.1.1

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=6cafe8445fd1e04e5f7d67bbc73029a538d1b253 # fio-3.31

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=d90b7d585067e87c56d8462b8e3e1c68996e2fc1

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=bab43553a904660266fdcd8fb974c7bdd96b3f58

FSVERITY_GIT=https://git.kernel.org/pub/scm/fs/fsverity/fsverity-utils.git
FSVERITY_COMMIT=5d6f7c4c2f82140207cf0300683217efe6cd0daa

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=00a0e66a14d3663edd9d37c8a01db6d182c88bdd # v1.3.2

BLKTESTS_GIT=https://github.com/tytso/blktests.git
BLKTESTS_COMMIT=676d42c9f932f4b6ca3f25439c2fc09d595ceefd

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
