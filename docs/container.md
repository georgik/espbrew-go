# Running espbrew in a container

This document covers running the espbrew cluster image as a **leader** with a
physical ESP32 device attached, and — importantly — the host prerequisite that
most people miss: **the serial device node must be world-accessible inside the
container**, which requires a small host-side `udev` rule.

## TL;DR

- The container must run **as your host user** via `--userns=keep-id`.
- The host serial device (`/dev/ttyACM0` or `/dev/ttyUSB0`) must be **world-readable/writable**
  (`0666`). On Linux this is done with a `udev` rule (see below). Without it,
  espbrew inside the container **cannot open the device**, even as root.
- The host user running podman/docker must itself be able to open the device
  (be a member of the `dialout` group on Linux).

## Why the device needs special handling

espbrew runs inside a **rootless** container. Rootless podman/docker always
creates a **user namespace** and maps the container's `agent`/`root` uid/gid
onto your host user. This is great for file ownership, but it has one side
effect for devices:

The serial node is owned `root:dialout` on the host. Inside the user namespace
the group `dialout` (gid 20) **cannot be represented** — it is not a mapped
subgid — so the node is remapped to `nobody:nogroup` and its group permission
bits are **no longer honoured** for the container process. Simply adding your
host user's groups with `--group-add=keep-groups` does **not** help: a process
inside the namespace still cannot open the device, even root cannot.

The only reliable way to let the container open the node is to make the node
**world-accessible** (`MODE 0666`) on the host. This is a deliberate, documented
trade-off for a locally-connected USB debug unit, and it is the same approach
used by many IoT/ESP flashing tools.

## Host prerequisite: make the device world-accessible

Create a `udev` rule so the node is set to `0666` whenever it is plugged in.

Create the file `/etc/udev/rules.d/69-espbrew-serial.rules`:

```
# Allow the container to open the ESP USB serial debug unit.
# Rootless user namespaces cannot represent the "dialout" group, so the node
# must be world-accessible for espbrew inside the container to open it.
SUBSYSTEM=="tty", ATTRS{idVendor}=="303a", MODE="0666", GROUP="dialout"
```

- `ATTRS{idVendor}=="303a"` matches Espressif devices. Adjust the vendor id if
  you use a different chip/FTDI/CP210x adapter.
- To match a specific board, you can additionally key on `ATTRS{idProduct}` or
  the serial string.

Then apply it (once):

```bash
sudo udevadm control --reload
sudo udevadm trigger --subsystem-match=tty
```

Verify the node is now `0666`:

```bash
stat -c '%n %U:%G %a' /dev/ttyACM0
# expected: /dev/ttyACM0 root:dialout 666
```

The rule re-applies automatically on every plug/reboot, so this is a one-time
setup. If you do not have passwordless `sudo` or `udev`, you can alternatively
`sudo chmod 0666 /dev/ttyACM0` before each launch, but the `udev` rule is the
persistent fix.

> Note: `udev` is Linux-specific. On macOS and Windows the device node is
> accessible to the running user directly, so no rule is required.

## State directory (HOME)

espbrew stores its state (database, bootloaders, captures) in `~/.espbrew`,
resolved from `$HOME` (`os.UserHomeDir()`). The image sets `HOME=/`, and under
`--userns=keep-id` the container process runs as your **host** user (not container
root), so `/` is not writable and espbrew aborts:

```
create espbrew directory: mkdir /.espbrew: permission denied
```

Two fixes:

1. **Quick / throwaway:** pass `-e HOME=/tmp`. State is lost on restart.

2. **Persistent (recommended for a leader):** bind-mount a state dir owned by
   your user. Create it first, then mount it and point `HOME` at it:

   ```bash
   mkdir -p espbrew-state
   podman run --rm -p 8080:8080 --userns=keep-id --group-add=keep-groups \
     --device=/dev/ttyACM0:rwm \
     -v "$(pwd)/espbrew-state":/state -e HOME=/state \
     ghcr.io/georgik/espbrew-go:latest
   ```

   The host dir must exist and be owned by your user before the run (otherwise
   podman creates it owned by root and the process cannot write to it).

**Image fix (permanent).** The image can be rebuilt so the default state dir is
writable, removing the need for either workaround. Add this line to the
Containerfile after the release binary is downloaded:

```dockerfile
RUN mkdir -p /.espbrew && chmod 777 /.espbrew
```

This creates `/.espbrew` at build time (as root) and makes it world-writable, so
it can be created by either uid 0 (bare run) or the mapped host user
(`keep-id` run).

**Why not run as a non-root user in the image.** You might be tempted to add a
dedicated `agent` user and `USER agent`. That would break native USB-hub power
control, which issues USB ioctls (`USBDEVIOCSPOWER`, etc.) that require root /
`CAP_SYS_RAWIO`. Keep the image running as root and use `--userns=keep-id`
instead.

## The run command

Linux — leader with a device attached:

```bash
podman run --rm -p 8080:8080 \
  --userns=keep-id \
  --group-add=keep-groups \
  --device=/dev/ttyACM0:rwm \
  ghcr.io/georgik/espbrew-go:latest
```

Notes on the flags:

- `--userns=keep-id` runs the container as your host user so file ownership is
  preserved. This is the important part for device access.
- `--group-add=keep-groups` carries your host user's supplementary groups. It is
  harmless and recommended, but **by itself it does not grant device access** —
  see above.
- `--device=/dev/ttyACM0:rwm` injects the node at the same path inside the
  container, granting read/write/mmap (needed for flashing).
- `--user=1000:1000` is **not** required and can be wrong if your host uid is
  not 1000. `--userns=keep-id` alone maps correctly to whatever your current
  user is. Avoid hard-coding `--user`.

macOS / Windows: same command without the `udev` prerequisite (the node is
already accessible to your user).

```bash
docker run --rm -p 8080:8080 \
  --userns=keep-id --group-add=keep-groups \
  --device=/dev/ttyACM0:rwm \
  ghcr.io/georgik/espbrew-go:latest
```

## Verifying device access from inside the container

Before trusting the setup, confirm the container can actually open the node.
Run an interactive shell override instead of espbrew:

```bash
podman run --rm -p 8080:8080 \
  --userns=keep-id --group-add=keep-groups \
  --device=/dev/ttyACM0:rwm \
  --entrypoint sh \
  ghcr.io/georgik/espbrew-go:latest -c '
    id
    test -w /dev/ttyACM0 && echo "DEVICE-WRITABLE" || echo "DEVICE-NOT-WRITABLE"
    printf "AT" > /dev/ttyACM0 && echo "WRITE-OK" || echo "WRITE-FAIL"
  '
```

Both `DEVICE-WRITABLE` and `WRITE-OK` should print. If the device shows
`nobody:nogroup` and the write fails, the host `udev` rule was not applied — see
troubleshooting below.

## Troubleshooting

### Device not found / "permission denied" inside the container

**Cause:** the node is not world-accessible, so the namespace cannot open it.

**Fix:**
1. Confirm the host user can open it: `sudo -n true; test -w /dev/ttyACM0`
   (as your normal user, not root).
2. Confirm the node is `0666`: `stat -c '%n %U:%G %a' /dev/ttyACM0`.
3. If it is `0660 root:dialout`, apply the `udev` rule above, or run
   `sudo chmod 0666 /dev/ttyACM0` as a temporary measure.

### `--user=1000:1000` does not help (or is wrong)

`--userns=keep-id` already maps the container onto your current host user.
Hard-coding `--user=1000:1000` only matches if your uid/gid are exactly 1000.
If your host user is different, drop `--user` and rely on `keep-id`.

### The device works on the host but not in the container

This is the user-namespace remapping described above. It is **not** fixed by
`--group-add=keep-groups` alone. The node must be world-accessible on the host.

### macOS build prerequisite (unrelated to devices)

The server uses cgo (via `pion/mediadevices` for cameras), so building on macOS
needs Apple's `clang` on `PATH`. If another tool (e.g. Swiftly) shadows it, the
build fails with `fatal error: 'stdlib.h' file not found`. See
[development.md](development.md).

## See also

- [README](../README.md) - Overview and quick start
- [platform-support.md](platform-support.md) - Windows/Linux specifics: stable
  device paths, COM ports, USB hub power control
- [power_control.md](power_control.md) - Native USB-hub power control
