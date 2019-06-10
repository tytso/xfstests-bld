ARCH_LIST=(amd64 i386 arm64)
MIRROR=http://snapshot.debian.org/archive/debian/20190610T085841Z/

#BEGIN CONFIG.CUSTOM

XFSTESTS_GIT=https://github.com/tytso/xfstests
XFSTESTS_COMMIT=a3cc922a340d487130d68e21a5775d02a24580d9

XFSPROGS_GIT=https://git.kernel.org/pub/scm/fs/xfs/xfsprogs-dev.git
XFSPROGS_COMMIT=65dcd3bc30ecda90728cd8ceceebc028f7feb47b # v5.0.0

FIO_GIT=http://git.kernel.dk/fio.git
FIO_COMMIT=a7760ecdb13394819b719f3f8181cc74c3d4affa # fio-3.14

QUOTA_GIT=https://git.kernel.org/pub/scm/utils/quota/quota-tools.git
QUOTA_COMMIT=daba90fb6d9b8c8f1361457bf2bea7b18f4e35ec

# SYZKALLER_GIT=https://github.com/google/syzkaller
# SYZKALLER_COMMIT=bab43553a904660266fdcd8fb974c7bdd96b3f58

FSVERITY_GIT=https://git.kernel.org/pub/scm/linux/kernel/git/ebiggers/fsverity-utils.git/
FSVERITY_COMMIT=bdebc45b4527d64109723ad5753fa514bac47c9f

IMA_EVM_UTILS_GIT=git://git.code.sf.net/p/linux-ima/ima-evm-utils.git
IMA_EVM_UTILS_COMMIT=0267fa16990fd0ddcc89984a8e55b27d43e80167

BLKTESTS_GIT=https://github.com/tytso/blktests.git
BLKTESTS_COMMIT=e689373c30a271b6a1fa7b45770dd306306ebd8a

NVME_CLI_GIT=https://github.com/linux-nvme/nvme-cli
NVME_CLI_COMMIT=4fe9563f8851cee4986d6f0d3bfcffc599e99fd4

#EXEC_LDFLAGS=-static
#EXEC_LLDFLAGS=-all-static
export PATH=$HOME/bin-ccache:/bin:/usr/bin
export CCACHE_DIR=/var/cache/ccache
export CCACHE_COMPRESS=t

BUILD_ENV="schroot -c $CHROOT --"
SUDO_ENV="schroot -c $CHROOT -u root --"
gen_image_args="--networking"
