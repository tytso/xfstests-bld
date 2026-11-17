# The kvm-xfstests skill

Many AI coding harness will attempt to test Linux kernels when testing a
kernel change, running a reproducer to validate a bug report, etc. in a
virtual machine.  Kvm-xfstests provides the ability for coding harneses
to be able to easily running xfstests.

It may also be useful for running reproducers in an environment where
basic utilities (Bash, python, coreutils, etc.)  are available in in a
VM environment, where previously the coding harness would create an
ad-hoc initramfs and then run qemu directly.

## 1. Installing the kvm-xfstests agent Skill file

The skill file is available in the Documentation folder as
[kvm-xfstests-SKILL.md](kvm-xfstests-SKILL.md).  To install the skill,
copy this file into the coding harness's agents skill directory as
`kvm-xfstests/SKILL.md`.

For `agy` (Antigravity CLI/IDE), Jetski, Codex CLI, and Gemini CLI, the
agents skill directory is `.agent/skills` in the root directory of the
workspace or the project, or the user's home directory.  If Claude Code
is used, it discovers skills from `.claude/skills/<name>/SKILL.md` so
symlinking avoids duplication, e.g:

  ```bash
  mkdir -p .claude/skills
  ln -s ../../.agents/skills/kvm-xfstests .claude/skills/kvm-xfstests
  ```

---

## 2. Harness-Agnostic Sandbox Requirements for `kvm-xfstests`

Many coding harness users may wish to enable to enable running a sandbox
when enabling YOLO mode (e.g., `--dangerously-skip-permissions`).   If
you don't think you need sandbox mode, you can stop reading now.

| Resource | Default path / primitive | Needed for | In-sandbox avoidance / fallback |
| :--- | :--- | :--- | :--- |
| **Read: wrappers & appliance** | `~/bin/{kvm-xfstests,install-kconfig,kbuild}`, `xfstests-bld` build tree (`run-fstests/`, `test-appliance/root_fs.img.*`, `kernel-build/`) | Launching `kvm-xfstests` and reading rootfs | Required (already readable in broad-read sandboxes; needs allowlisting in strict allowlist sandboxes on `/home/<user>` or `/Users/<user>` hosts) |
| **Read: user config** | `~/.config/kvm-xfstests` (+ symlink target if dotfile-managed; optional `~/.config/xfstests-common`, `~/.local/etc/kvm-xfstests`) | Default `KERNEL`, `TZ`, disk/LVM settings | Skipped when `XFSTESTS_CONFIG` and `--kernel` are set |
| **Read/Write: external kernel build** | `BLD_DIR` (e.g., `/build/ext4`; parent `/build` if `kbuild` builds `.deb`s) | Booting default kernel (R/O) or running `kbuild` (R/W) | Build out-of-tree into a writable workspace/scratch dir (`O=<scratch>/build`) and pass `--kernel` |
| **Write: rootfs snapshot overlay** | `/var/tmp/vl.XXXXXX` | QEMU `-drive ...,snapshot=on` | Set `TMPDIR=<writable-dir>/tmp` (must **not** be literal `/tmp`) |
| **Write: system `/tmp`** | `/tmp/xfstests-cli.*` (`vdh`), `/tmp/kvm-xfstests-$USER`, `/tmp/files.*`, `/tmp/log.*` | Transfer tarball, default 9p dir, exit-code extraction | Allowed by default in most sandboxes; pass `--virtfs <scratch>/vtmp` to isolate 9p share |
| **Write: scratch disks & logs** | `$KVM_XFSTESTS_DIR/disks/vdb..vdj`, `$KVM_XFSTESTS_DIR/logs` | Shared test disks and persistent logs/`--archive` | Redirect disks to a disk-backed (ext4/xfs) dir via `XFSTESTS_CONFIG` (or add `--cache default` on `tmpfs`); skip logs with `--no-log` |
| **Devices & syscalls** | `/dev/kvm` R/W + `kvm` supplementary group (macOS arm64: `accel=hvf`), `/dev/urandom`, `/dev/ptmx`+`/dev/pts` (for `expect`), Linux AIO (`io_setup`/`io_submit`) | Hardware VM acceleration, console automation, `cache=none,aio=native` | Missing `/dev/kvm` falls back to slow TCG (`accel=kvm:tcg`); blocked Linux AIO → `--aio threads`; avoid `--aio io_uring` (commonly seccomp-blocked) |
| **Loopback TCP bind** | `localhost:7498` (monitor), `7499` (gdb), `7500..7502` (serial) | Optional debug/serial listeners (`-net none` needs no outbound net) | Set `MONITOR=""`, `GDB=""`, `SERIAL=""` in `~/.config/kvm-xfstests` or `XFSTESTS_CONFIG` |

---

## 3. Harness-Specific Configuration

### A. Antigravity (`agy` CLI & IDE) and Jetski

Both use the same namespace/Seatbelt sandbox (`exebox`) and `action(target)` rule syntax. Paths must be **literal and absolute** (no `~` or globs; symlinks are resolved automatically).

- **Config files** (or configure interactively via `/config` and `/permissions`):
  - **`agy` (Antigravity CLI):** `~/.gemini/antigravity-cli/settings.json`
  - **Jetski CLI:** `~/.gemini/jetski/cli/settings.json`
  - **Shared across Antigravity IDE, Jetski Hub, and both CLIs:** `~/.gemini/config/config.json` under `userSettings.globalPermissionGrants` (note: `enableTerminalSandbox` is still toggled separately in each CLI's `settings.json`; per-project rules live in `~/.gemini/config/projects/<id>.json`).
- **Behavior notes:**
  - **Linux `/dev/kvm` works natively:** `/dev` is mounted R/W, the caller's `kvm` supplementary group is preserved, Linux AIO is allowed, and loopback binds succeed inside the isolated network namespace.
  - **`$HOME` paths need explicit `read_file(...)`:** On a standard `/home/<user>` or macOS `/Users/<user>` machine, `~/bin` and `~/xfstests-bld` are **not** readable unless granted.
  - **macOS (`accel=hvf`):** Seatbelt restricts IOKit/Hypervisor calls; add `"unsandboxed(kvm-xfstests)"` if hardware acceleration is blocked.

Example `~/.gemini/antigravity-cli/settings.json` (or `~/.gemini/jetski/cli/settings.json`), substituting `/home/<user>`:

```json
{
  "enableTerminalSandbox": true,
  "toolPermission": "proceed-in-sandbox",
  "permissions": {
    "allow": [
      "read_file(/home/<user>/bin)",
      "read_file(/home/<user>/xfstests-bld)",
      "read_file(/home/<user>/.config/kvm-xfstests)",
      "write_file(/var/tmp)",
      "write_file(/home/<user>/xfstests-bld/build-64/run-fstests/disks)",
      "write_file(/home/<user>/xfstests-bld/build-64/run-fstests/logs)",
      "write_file(/build)",
      "command(kvm-xfstests)",
      "command(install-kconfig)",
      "command(kbuild)"
    ]
  }
}
```

*(If `~/.config/kvm-xfstests` is a symlink, granting the symlink resolves to its target automatically. To keep shared `disks/`, `logs/`, and `/var/tmp` untouched, drop those three `write_file(...)` entries and rely on the skill's `TMPDIR` + `XFSTESTS_CONFIG` + `--no-log` pattern; change `write_file(/build)` to `read_file(/build/ext4)` if agents should only boot prebuilt kernels.)*

For `~/.gemini/config/config.json`, place the same `allow` array under `userSettings.globalPermissionGrants.allow` with `"enableTerminalSandbox": true`.

---

### B. Claude Code

Configured in `~/.claude/settings.json` (user), `.claude/settings.json` (shared project), or `.claude/settings.local.json` (local, git-ignored), or via `/sandbox`. Claude Code uses `@anthropic-ai/sandbox-runtime` (`bubblewrap` on Linux/WSL2, Seatbelt on macOS): reads are broadly allowed by default (unless in `denyRead`), while writes are limited to CWD and `/tmp`. In `sandbox.filesystem.*`, leading `/` is an absolute path and `~/` expands `$HOME` (directories must already exist on Linux).

**Critical `/dev/kvm` limitation on Linux:** `bubblewrap` is invoked with `--dev /dev`, which mounts a minimal synthetic `tmpfs` (with `nodev`) that omits `/dev/kvm`. Adding `"/dev/kvm"` to `allowWrite` uses plain `--bind` rather than `--dev-bind`, so `nodev` remains in effect and KVM still fails. That leaves two options:

**Option 1 — Fully sandboxed (falls back to software TCG on Linux; fine for `ver`/small reproducers, slow for `smoke`):**

```json
{
  "sandbox": {
    "enabled": true,
    "autoAllowBashIfSandboxed": true,
    "filesystem": {
      "allowWrite": [
        "/var/tmp",
        "~/xfstests-bld/build-64/run-fstests/disks",
        "~/xfstests-bld/build-64/run-fstests/logs",
        "/build"
      ]
    },
    "network": {
      "allowLocalBinding": true
    }
  }
}
```

*(`network.allowLocalBinding` is mainly needed on macOS if `MONITOR`/`GDB`/`SERIAL` are not cleared.)*

**Option 2 — Native KVM / macOS HVF speed via `excludedCommands` (recommended for test suites):**

```json
{
  "permissions": {
    "allow": [
      "Bash(kvm-xfstests *)"
    ]
  },
  "sandbox": {
    "enabled": true,
    "autoAllowBashIfSandboxed": true,
    "excludedCommands": [
      "kvm-xfstests *"
    ],
    "filesystem": {
      "allowWrite": [
        "/build"
      ]
    }
  }
}
```

*Caveat:* `excludedCommands` matches the literal command string—chained commands (`cmd && kvm-xfstests ...`), leading `VAR=val` prefixes, or `expect -c 'spawn kvm-xfstests ...'` stay inside the sandbox unless invoked as a standalone `kvm-xfstests ...` command (or wrapped in a dedicated helper script added to `excludedCommands`).

---

### C. Other Harnesses (Codex CLI, Gemini CLI, Custom Sandboxes)

- **OpenAI Codex CLI (`~/.codex/config.toml`):** Reads are allowed under `sandbox_mode = "workspace-write"`; grant extra write paths under `[sandbox_workspace_write]` via `writable_roots = ["/var/tmp", "/home/<user>/xfstests-bld/build-64/run-fstests/disks", "/home/<user>/xfstests-bld/build-64/run-fstests/logs", "/build"]` (or pass `--add-dir`). Like Claude Code, Codex's `bwrap` Linux sandbox strips `/dev/kvm` (falling back to TCG unless the run is approved outside the sandbox).
- **Gemini CLI (`~/.gemini/settings.json` / `GEMINI_SANDBOX`):** Container sandboxes (`docker`/`podman`) require passing `--device /dev/kvm` and bind-mounting `xfstests-bld` and the kernel build tree into a container image that includes `qemu-system-*` (via `SANDBOX_FLAGS` / `SANDBOX_MOUNTS`). macOS `sandbox-exec` mode requires either a custom `.gemini/sandbox-macos-<profile>.sb` profile or running `kvm-xfstests` unsandboxed.
- **Custom `bwrap` / `nsjail` wrappers:** Keep `--dev /dev` for general device isolation and add `--dev-bind /dev/kvm /dev/kvm` (not `--bind`), retain the caller's `kvm` supplementary group, allow `io_setup`/`io_submit`/`io_getevents`/`io_destroy` in `seccomp` (or pass `--aio threads`), and either bind `/var/tmp` R/W or set `TMPDIR` to a non-`/tmp` writable directory.

