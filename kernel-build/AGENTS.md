# kernel-build

This directory contains tools and configurations for building Linux
kernels compatible with the xfstests test appliance.

## Key Scripts

- `install-kconfig`: Merges base kernel configurations with debug/feature fragments and installs them as `.config` in the kernel build directory. Must be run from the top-level of kernel sources.
- `kbuild`: Orchestrates the kernel build process, supporting out-of-tree builds, Debian package creation, and module packaging.

## Configuration

`kbuild` reads its configuration from the kernel source tree it is operating on:
- Location: `$GIT_DIR/kbuild/config` (or `$GIT_DIR/kbuild.conf` as fallback).
- It is recommended to create this file in your kernel source tree to define the build directory and target architecture.

Example `$GIT_DIR/kbuild/config`:
```bash
BLD_DIR=/path/to/external/build/dir
KERN_ARCH=x86_64
# Arch-specific build dirs can also be defined:
# BLD_DIR_ARM64=/path/to/arm64/build
# BLD_DIR_I386=/path/to/i386/build
```

## Kernel Configurations

The directory `kernel-configs` contains base configurations and feature fragments:
- **Base Configs**: Named `config-<version>` (e.g., `config-6.1`) or `<arch>-config-<version>` (e.g., `arm64-config`).
- **Feature Fragments**:
  - `blktests-configs`: Configs needed for running blktests.
  - `extra-debug-configs`: General debugging options.
  - `full-debug-info-configs`: Enables debug info.
  - `kasan-configs` / `kcsan-configs` / `ubsan-configs`: Kernel sanitizers.
  - `lockdep-configs`: Lock dependency validator.
  - `dept-configs`: Dependency tracker.

## Workflows

### 1. Preparing Kernel Configuration (`install-kconfig`)

Run this from the top-level of your kernel source tree:
```bash
/path/to/kernel-build/install-kconfig [--arch <arch>] [<options>]
```
Options:
- `--arch <arch>`: Target architecture (e.g., `amd64` (default), `i386`, `arm64`).
- `--debug` or `--extra-debug`: Enable debug options (uses `extra-debug-configs`).
- `--debug-info` or `--full-debug-info`: Enable full debug info.
- `--kasan` / `--kcsan` / `--ubsan` / `--lockdep`: Enable respective sanitizers/checkers.
- `--blktests`: Enable configs for blktests.
- `--perf`: Use performance-oriented base config.

This script determines the kernel version of your source tree, finds the closest matching base config, merges it with selected fragments, writes it to `.config` in the build directory, and runs `make olddefconfig`.

### 2. Building the Kernel (`kbuild`)

Run this from the top-level of your kernel source tree:
```bash
/path/to/kernel-build/kbuild [<options>] [<make-arguments>]
```
Options:
- `--arch <arch>`: Override target architecture.
- `--install-kconfig`: Run `install-kconfig` before building.
- `--install-kconfig-opts "<opts>"`: Pass options to `install-kconfig`.
- `--oldconfig`: Run `make olddefconfig` before building.
- `--dpkg`: Build Debian packages (default if no extra target is specified).
- `--no-dpkg`: Build raw kernel image and modules (no deb packages).
- `-j <jobs>`: Number of parallel make jobs (defaults to CPU count).

#### Build Outputs (located in the configured `BLD_DIR`):
- **With `--dpkg`**:
  - `linux-image.deb`: Kernel image package.
  - `linux-image-dbg.deb`: Debug symbols package.
  - `linux-headers.deb`: Kernel headers package.
- **With `--no-dpkg`**:
  - Raw kernel image (e.g., `arch/x86/boot/bzImage`).
  - `modules.tar.xz`: Compressed tarball of installed modules (automatically generated if `CONFIG_MODULES=y`). This is used by `kvm-xfstests` to update modules in the VM.
  - `.git_version`: Contains output of `git describe` for the built commit.
