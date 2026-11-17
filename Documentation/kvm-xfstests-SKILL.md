---
name: kvm-xfstests
description: >-
  Builds, boots, and tests Linux kernels in a lightweight QEMU/KVM virtual
  machine using kvm-xfstests, install-kconfig, and kbuild (from xfstests-bld)
  when available. Use when reproducing kernel or filesystem bugs and crashes
  (including syzkaller reproducers), booting a test kernel into a VM root
  shell to run custom reproducers or inspect behavior, or running xfstests
  and blktests regression tests (individual tests, groups, filesystem
  configs like inline/1k/data_journal, or smoke runs) instead of constructing
  custom QEMU scripts, disk images, or initramfs archives from scratch.
  Don't use for pure KUnit unit tests that run under UML without block
  devices, or for AI code review submission (use sashiko).
---

# Kernel & Filesystem VM Testing with `kvm-xfstests`

When `kvm-xfstests` is installed on the host, use it instead of hand-crafting
custom QEMU command lines, `initramfs` archives, static PID-1 programs, or ad-hoc
disk images. `kvm-xfstests` boots a prebuilt Debian test appliance in seconds
with KVM acceleration, pre-attached virtio scratch disks, a host-shared 9p
directory (`/vtmp`), filesystem utilities (`e2fsprogs`, `xfs_io`, `btrfs-progs`,
`f2fs-tools`, `quota`, `dmsetup`, `keyctl`), `python3`/`perl`, unprivileged test
users (`fsgqa`, `fsgqa2`), and the full `xfstests` suite.

## 1. Check Availability & Inspect Configuration

Before setting up any VM test environment, check whether `kvm-xfstests` is on
`PATH` (commonly `~/bin/kvm-xfstests`):

```bash
command -v kvm-xfstests
kvm-xfstests --help
```

- If `kvm-xfstests` is **not** installed, fall back to KUnit (`kunit.py`) or a
  minimal QEMU setup.
- Use `--no-action` on any `kvm-xfstests` invocation to print the exact
  `qemu-system-*` command line and resolved kernel path without starting a VM:
  ```bash
  kvm-xfstests --no-action --kernel /path/to/bzImage shell
  ```

---

## 2. Building a Test Kernel

Plain `defconfig` often omits options needed by the test appliance and
`xfstests` (such as `CONFIG_NET_9P`, `CONFIG_NET_9P_VIRTIO`, and `CONFIG_9P_FS`
for `/vtmp`, or quota, fscrypt, fsverity, and `dm-flakey` targets). Prefer the
`xfstests-bld` kernel config fragments when building a kernel for VM testing.

### Option A: Standard `install-kconfig` + `kbuild` (when modifying the active build tree is OK)

Run from the top level of the kernel source tree:

```bash
# Generate a trimmed appliance-ready .config with desired debug/sanitizer fragments:
install-kconfig --lockdep --kasan --extra-debug

# Build raw kernel image + modules.tar.xz (skip slow .deb packaging):
kbuild --no-dpkg
```

Useful `install-kconfig` flags: `--extra-debug`, `--lockdep`, `--kasan`,
`--ubsan`, `--kcsan`, `--dept`, `--full-debug-info`, `--blktests`, `--perf`,
`--clang`, `--i386`, `--arm64`.

### Option B: Isolated Scratch Build (when preserving the user's checkout & `BLD_DIR`)

`install-kconfig` and `kbuild` operate on the `BLD_DIR` configured in
`$GIT_DIR/kbuild/config` and update `$GIT_DIR/kbuild/kernel-config`. When you
must keep the user's checkout and primary build directory untouched (e.g., testing
in a scratch `git worktree`):

1. Seed `<scratch>/build/.config` from the appliance base config (locate it via
   `install-kconfig --get-config-fn`, or copy `$GIT_DIR/kbuild/kernel-config` if
   present) and merge any needed fragments from the same `kernel-configs/`
   directory (`kasan-configs`, `lockdep-configs`, `extra-debug-configs`).
2. Build out-of-tree with `make O=<scratch>/build olddefconfig` and
   `make O=<scratch>/build -j$(nproc) bzImage` (if `CONFIG_MODULES=y`, either
   build modules or set `CONFIG_EXT4_FS=y` / built-in subsystems as needed).
3. Pass `--kernel <scratch>/build/arch/x86/boot/bzImage` (or
   `--kernel <scratch>/build`, from which `kvm-xfstests` automatically picks
   `arch/x86/boot/bzImage` or `arch/arm64/boot/Image` and `modules.tar.xz` if
   present).

---

## 3. Running `xfstests` and Smoke Suites

`kvm-xfstests` test runs are non-interactive: the VM boots, runs the requested
tests, saves logs, and powers off automatically.

```bash
# Run individual tests under a specific filesystem configuration:
kvm-xfstests --kernel /path/to/bzImage -c 4k ext4/001 generic/068

# Test specific ext4 configurations (comma-separated list allowed with single -c):
# Common configs: 4k, 1k, inline, inline_1k, data_journal, nojournal, ext3,
# ext3conv, bigalloc_4k, bigalloc_1k, encrypt, fast_commit, quota, dioread_nolock
kvm-xfstests --kernel /path/to/bzImage -c inline,1k -g quick

# Pass extra mount options (-m) or disable auto-excludes (-a):
kvm-xfstests --kernel /path/to/bzImage -c 4k -m "errors=remount-ro" -a ext4/057

# Run pre-submission smoke suite (~15-20 minutes; set appropriate timeout):
kvm-xfstests --kernel /path/to/bzImage smoke
```

- **Summarizing logs:** By default, output is tee'd to
  `$KVM_XFSTESTS_DIR/logs/log.<DATECODE>` (pass `--no-log` when you only want
  stdout and no persistent log file). `get-results` lives in the same
  `run-fstests/` directory that the `kvm-xfstests` wrapper sets as
  `KVM_XFSTESTS_DIR` (inspect `$(command -v kvm-xfstests)` for `DIR=`):
  ```bash
  /path/to/run-fstests/get-results --failures /path/to/run-fstests/logs/log.<DATECODE>
  ```
  Pass `--archive` to `kvm-xfstests` if you need `/results` (`*.out.bad`,
  `*.full`, `dmesg`) exported to `logs/results-<DATECODE>.tar.xz`.
- **Avoid `kvm-xfstests full` locally** unless explicitly requested by the user;
  a full multi-config run takes ~24 hours (`gce-xfstests` is intended for full
  runs).

---

## 4. Booting into `shell` for Custom Reproducers & Crash Testing

To reproduce a bug report, test edge cases (`ENOSPC`, `EDQUOT`, mount options,
crash/journal replay), or inspect live kernel state, boot into the appliance
shell with:

```bash
kvm-xfstests --no-log --kernel /path/to/bzImage --virtfs /path/to/vtmp shell
```

### Appliance Environment Reference

- **Ephemeral rootfs (`/dev/vda`):** `shell` boots `root_fs.img` with QEMU
  `snapshot=on`, so `/` changes are discarded when QEMU exits. **Never** run
  `kvm-xfstests maint` in place of `shell` — `maint` disables `snapshot=on` and
  permanently modifies the shared `root_fs.img` appliance image.
- **Host↔Guest shared directory (`/vtmp`):** Host `/tmp/kvm-xfstests-$USER` (or
  the directory passed to `--virtfs <dir>`) is mounted read-write at `/vtmp`
  inside the guest via 9p (`9p2000.L`). Always prefer `--virtfs <scratch>/vtmp`
  to keep reproducers and output files isolated per conversation.
- **Compile C reproducers on the host, not in the guest:** The appliance
  includes `bash`, `python3`, `perl`, `xfs_io`, `mke2fs`, `debugfs`, `e2fsck`,
  `tune2fs`, `dumpe2fs`, `setquota`, `quotaon`, `keyctl`, `dmsetup`, `losetup`,
  `strace`, and `/root/xfstests`, **not** `gcc`. Compile C test binaries on the
  host into `<scratch>/vtmp/` (add `-static` only if host `glibc` is newer than
  the appliance's) and run `/vtmp/<binary>` in the guest.
- **Scratch block devices:**
  - `/dev/vdb` (5 GiB), `/dev/vdc` (5 GiB), `/dev/vdd` (5 GiB)
  - `/dev/vde` (20 GiB), `/dev/vdf` (20 GiB)
  - `/dev/vdi` (1 GiB), `/dev/vdj` (1 GiB)
  - `/dev/vdg` (5 GiB, mounted at `/results`) and `/dev/vdh` (internal transfer
    tarball) — leave `/dev/vdg` and `/dev/vdh` alone during custom shell tests;
    use `/dev/vdc`..`/dev/vdf` or `/dev/vdi`..`/dev/vdj` (and mount points such
    as `/mnt/scratch` or `/mnt/test`).
  - Unprivileged users `fsgqa` (UID 31415) and `fsgqa2` (UID 31416) exist out of
    the box for permission/quota tests.

### Driving `shell` Non-Interactively from Agent Tool Calls

On boot, `serial-getty@ttyS0` auto-logs in `root` and `/root/.bashrc` executes
`/root/xfstests/bin/resize`, which sends an ANSI cursor-position query (`\e[6n`)
and waits ~5 seconds before printing `root@kvm-xfstests:~#`. Do **not** pipe
commands directly into `kvm-xfstests ... shell` stdin at startup — bytes sent
before the prompt appears are swallowed by `agetty`/`resize`.

#### Pattern 1: Single-Shot Scripted Run with `expect` (Recommended)

Write your guest script to `<scratch>/vtmp/run.sh` (`chmod +x`), write structured
results to `/vtmp/result.txt` (or print to serial console), and run via `expect`:

```bash
expect -c '
  set timeout 90
  spawn kvm-xfstests --no-log --kernel /path/to/bzImage --virtfs /path/to/vtmp -o "oops=panic panic=-1" shell
  expect {
    "\x1b\[6n"          { send "\x1b\[24;80R"; exp_continue }
    "root@kvm-xfstests" { send "/vtmp/run.sh; poweroff -f\r" }
    "Kernel panic"      { send "\x01x"; exit 2 }
    timeout             { send "\x01x"; exit 3 }
  }
  expect {
    eof                 { exit 0 }
    "Kernel panic"      { send "\x01x"; exit 2 }
    timeout             { send "\x01x"; exit 3 }
  }
'
```

Why this pattern works reliably:
- Replying `\x1b[24;80R` to `\x1b[6n` satisfies `resize` immediately without
  waiting 5 seconds for its timeout.
- `-o "oops=panic panic=-1"` turns kernel Oopses into immediate panics/exits
  instead of continuing with corrupted state.
- Sending `\x01x` (`Ctrl-a x`) tells QEMU's `mon:stdio` to terminate immediately
  if the guest panics or deadlocks with interrupts/preemption disabled.

#### Pattern 2: Interactive Background Session

When debugging interactively across multiple steps, launch
`kvm-xfstests --no-log --kernel /path/to/bzImage --virtfs /path/to/vtmp shell`
as a background task or PTY session, wait until `root@kvm-xfstests:~#` appears
in its output, write commands to its stdin, and shut down with `poweroff -f`
(or `\x01x`). Inside an interactive shell you can also run `xfstests` subsets
directly:

```bash
FSTESTCFG="4k inline" FSTESTSET="ext4/001 generic/001" /root/runtests.sh
```

---

## 5. Gotchas & Concurrency

- **Per-run isolated disks, snapshot dir & ports (`XFSTESTS_CONFIG` + `TMPDIR`):**
  By default, every invocation shares `$KVM_XFSTESTS_DIR/disks/vdb..vdj` (opened
  read-write with `cache=none,aio=native`), writes its `snapshot=on` overlay to
  `/var/tmp/vl.XXXXXX`, and binds `localhost:7498..7502`. When another VM is
  already running, when running parallel VMs across subagents, or when sandboxed
  writes are confined to a workspace/scratch directory, allocate per-run sparse
  disks and a `TMPDIR` subdirectory in a **disk-backed** (e.g., ext4/xfs, not
  `tmpfs`) writable directory `$SCRATCH`:
  ```bash
  mkdir -p "$SCRATCH/tmp" "$SCRATCH/vtmp" "$SCRATCH/disks"
  for d in vdb:5g vdc:5g vdd:5g vde:20g vdf:20g vdg:5g vdi:1g vdj:1g; do
    qemu-img create -q -f raw "$SCRATCH/disks/${d%:*}" "${d#*:}"
  done
  /sbin/mke2fs -q -t ext4 -F "$SCRATCH/disks/vdb"
  /sbin/mke2fs -q -t ext4 -F "$SCRATCH/disks/vdg"
  cat > "$SCRATCH/kvm.conf" <<EOF
  VDB=$SCRATCH/disks/vdb
  VDC=$SCRATCH/disks/vdc
  VDD=$SCRATCH/disks/vdd
  VDE=$SCRATCH/disks/vde
  VDF=$SCRATCH/disks/vdf
  VDG=$SCRATCH/disks/vdg
  VDI=$SCRATCH/disks/vdi
  VDJ=$SCRATCH/disks/vdj
  SERIAL=""
  GDB=""
  MONITOR=""
  EOF
  TMPDIR="$SCRATCH/tmp" XFSTESTS_CONFIG="$SCRATCH/kvm.conf" \
    kvm-xfstests --no-log --virtfs "$SCRATCH/vtmp" --kernel /path/to/bzImage ...
  ```
  Always clean up `$SCRATCH` when finished.
- **`kvm-xfstests syz <repro>` in out-of-tree `xfstests-bld` builds:** `syz`
  copies `$KVM_XFSTESTS_DIR/test-appliance/syz`, which exists in in-tree builds
  but may live at `$(dirname "$KVM_XFSTESTS_DIR")/test-appliance/syz` in
  separate build directories (such as `build-64/run-fstests`). If `syz` reports
  that directory missing, compile the reproducer on the host and run it via
  `--virtfs` + `shell`.

---

## 6. Running Inside Agent Sandboxes

OS-level terminal sandboxes (used by harnesses such as Antigravity/`agy`, Claude
Code, Codex CLI, Gemini CLI, and Jetski via Linux namespaces/`bubblewrap`/
`nsjail`/Landlock or macOS Seatbelt `sandbox-exec`) commonly restrict writes,
reads, `/dev` nodes, syscalls, and loopback sockets. Prefer the isolated
`TMPDIR="$SCRATCH/tmp" XFSTESTS_CONFIG="$SCRATCH/kvm.conf" ... --no-log --virtfs "$SCRATCH/vtmp"`
pattern from Section 5 when sandboxed, which confines disk, overlay, log, and 9p
writes to `$SCRATCH` (plus small helper directories under `/tmp`).

Common sandbox failure modes and remedies:

- **`Could not open temporary file '/var/tmp/vl.XXXXXX': Read-only file system`:**
  QEMU's `snapshot=on` queries `TMPDIR` (via `g_get_tmp_dir()`), but explicitly
  rewrites a literal `"/tmp"` to `"/var/tmp"` to avoid filling `tmpfs`. Set
  `TMPDIR="$SCRATCH/tmp"` (any writable directory **other** than literal `/tmp`,
  preferably disk-backed), or ask the user to allow writes to `/var/tmp` in
  their sandbox config. **Never** run `maint` to bypass `snapshot=on`.
- **Missing `/dev/kvm` (`failed to initialize kvm` / `Back to tcg accelerator`)
  or blocked macOS `hvf`:** Sandboxes that mount a minimal synthetic `/dev`
  (e.g., `bubblewrap --dev /dev` in Claude Code or strict `bwrap` modes in Codex
  CLI) hide `/dev/kvm`, and standard `--bind` path allowlists retain `nodev`. On
  x86 Linux, `kvm-xfstests` passes `accel=kvm:tcg`, so QEMU falls back to slow
  software TCG emulation (usable with longer timeouts for `ver` or a quick
  reproducer, impractical for `smoke` or multi-test runs). On macOS (`accel=hvf`),
  Seatbelt blocks Hypervisor.framework outright. When hardware acceleration is
  needed, re-run `kvm-xfstests` outside the sandbox via the harness's user
  approval flow (or a user-configured sandbox exemption rule).
- **Unreadable `~/bin`, `xfstests-bld`, `~/.config/kvm-xfstests`, or external
  `BLD_DIR`:** Strict allowlist sandboxes (such as `agy`/Antigravity and Jetski)
  deny `~/.config` and non-workspace directories (including `/home/<user>/bin`,
  `~/xfstests-bld`, and external build trees like `/build`) unless explicitly
  granted read access (along with any symlink targets for dotfile configs).
  Report the missing path so the user can allowlist it, or request an
  unsandboxed run.
- **`O_DIRECT` (`EINVAL`) or AIO errors on scratch disks:** Default
  `cache=none,aio=native` rejects `tmpfs` (place `$SCRATCH/disks` on a
  disk-backed filesystem, or pass `--cache default`) and uses Linux AIO syscalls
  (`io_setup`/`io_submit`; pass `--aio threads` if `seccomp` blocks kernel AIO).
  Avoid `--aio io_uring` in sandboxes, which usually deny `io_uring_*`.
- **Loopback port bind errors (`localhost:7498..7502`):** `config.kvm` listens
  on TCP ports `7498` (monitor), `7499` (gdb), and `7500..7502` (serial). If the
  sandbox forbids local `bind()` (for example, macOS Seatbelt without local
  binding permitted), clear `MONITOR=""`, `GDB=""`, and `SERIAL=""` via
  `XFSTESTS_CONFIG`.
- **Container-based sandboxes (e.g., Docker/Podman):** Only work if the host's
  `xfstests-bld` tree, kernel image, and `/dev/kvm` (`--device /dev/kvm`) are
  mounted into the container and `qemu-system-*` is installed inside it;
  otherwise ask the user to run `kvm-xfstests` on the host.
