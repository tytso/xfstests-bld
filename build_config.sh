ARCH_LIST=(amd64 i386 arm64)
MIRROR=http://snapshot.debian.org/archive/debian/20190826T092742Z/

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=b15701bcefccc59b519b5a8d26c0bcf07751cb22 # 2020-03-19 release

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=0cfb2952319d237f1d079097810546f24e3883bf # v5.4.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=ac694f66968fe7b18c820468abd8333f3df333fb # fio-3.18

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=9a001cc6eb211758015d85cecc0464c94c82bbb5

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=bab43553a904660266fdcd8fb974c7bdd96b3f58

FSVERITY_GIT=https://git.kernel.org/pub/scm/linux/kernel/git/ebiggers/fsverity-utils.git/
FSVERITY_COMMIT=6585eb4968a0f3f0811bd8707ff5b04c78cf1c5e # v1.0

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=515c99856ef52bbf680e6dd6c338acfb8d088614 # v1.2

BLKTESTS_GIT=https://github.com/tytso/blktests.git
BLKTESTS_COMMIT=f7b47c58e87caf91e1648b3c0fa348c555595800

NVME_CLI_GIT=https://github.com/linux-nvme/nvme-cli
NVME_CLI_COMMIT=1d84d6ae0c7d7ceff5a73fe174dde8b0005f6108 # v1.10.1

UTIL_LINUX_GIT=https://git.kernel.org/pub/scm/utils/util-linux/util-linux.git
UTIL_LINUX_COMMIT=d35573591278a231a62cefdbee5b0beb0fb62dc8 # v2.35
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
