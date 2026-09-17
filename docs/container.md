# Running espbrew in a container

The published image `ghcr.io/georgik/espbrew-go:latest` bundles the pre-built release binary and runs espbrew in cluster leader mode, exposing the dashboard and API on port 8080. The image is intended for a single machine that has the ESP32 devices physically attached to it.

This document covers running the image as a **leader** with a physical ESP32 device attached — and importantly the host prerequisite that most people miss: **the serial device node must be reachable inside the container**, which on Linux is most reliably done with a small host-side `udev` rule (or by running the container as your host user).

## TL;DR

- With **no device mounted**, the node starts cleanly but reports zero real devices. Open http://localhost:8080 to confirm the dashboard is up — no special flags needed.
- The container must run **as your host user** via `--userns=keep-id` to reach a device.
- The host serial device (`/dev/ttyACM0` or `/dev/ttyUSB0`) must be reachable: either **world-accessible** (`0666`, via a `udev` rule — see below) or the container must run as the mapped host user with the host's groups (`--group-add=keep-groups`).
- The host user running podman/docker must itself be able to open the device (be a member of the `dialout` group on Linux).
- State lives in `~/.espbrew`; point `HOME` at a writable/bind-mounted dir to persist it across restarts.

## Base command (no device)

```bash
# podman
podman run --rm -p 8080:8080 ghcr.io/georgik/espbrew-go:latest

# docker
docker run --rm -p 8080:8080 ghcr.io/georgik/espbrew-go:latest
```

With **no device mounted**, the node starts cleanly but reports zero real devices (there is nothing inside the container to talk to). Open http://localhost:8080 to confirm the dashboard is up. No special flags are needed in this case.

## Mounting a serial device

To let espbrew discover, monitor, and flash the ESP32 attached to your host, run the container **as your host user** and mount the device node. This is the same recipe `orchestrion shell` produces for the agent image:

```bash
podman run --rm \
  --userns=keep-id \
  --user=1000:1000 \
  --group-add=keep-groups \
  --device=/dev/ttyACM0:rwm \
  -p 8080:8080 \
  ghcr.io/georgik/espbrew-go:latest
```

- `--userns=keep-id --user=1000:1000` runs the container as the host user (uid/gid `1000` here — run `id -u` / `id -g` to match the host you run on). This is **required** to reach the device, because a rootless container remaps the mounted node to `nobody:nogroup` and root cannot open it — see below.
- `--group-add=keep-groups` maps the host user's supplementary groups into the container, so the remapped device's group permission bits are honoured.
- `--device=/dev/ttyACM0:rwm` injects the node at the same path inside the container. The `:rwm` grants read/write/mmap — espbrew needs write to flash; a plain `--device=/dev/ttyACM0` inherits the node's `0660` mode and also allows write via the group, so it works too.
- The node is usually `/dev/ttyACM0` (USB CDC) or `/dev/ttyUSB0` (FTDI/CH340/CP210x). espbrew additionally resolves stable `/dev/serial/by-id/` symlinks.
- `podman` and `docker` accept the same flags; use whichever host tool you use.

### Running an interactive shell (diagnostics)

For diagnostics — inspect the device, run `espbrew devices`, open a REPL — start the container with `/bin/sh` as the entrypoint, applying the same host-user / keep-id flags:

```bash
podman run --rm -it \
  --userns=keep-id \
  --user=1000:1000 \
  --group-add=keep-groups \
  --entrypoint /bin/sh \
  --device=/dev/ttyACM0:rwm \
  -p 8080:8080 \
  ghcr.io/georgik/espbrew-go:latest
```

Then, inside the shell:

```bash
ls -l /dev/ttyACM0      # readable/writable via your group
espbrew devices         # list the devices espbrew can see
```

If you instead want to run espbrew's CLI directly inside a throwaway container, pass the subcommand after the image (no `-it`, no shell needed):

```bash
podman run --rm \
  --userns=keep-id --user=1000:1000 --group-add=keep-groups \
  --device=/dev/ttyACM0:rwm \
  -e HOME=/tmp \
  ghcr.io/georgik/espbrew-go:latest devices
```

## Why the container must run as the host user

The image declares no `USER`, so it defaults to **root** inside the container. You might expect root to access any device, but on a **rootless** podman/docker the story is different.

espbrew runs inside a rootless container. Rootless podman/docker always creates a **user namespace** and maps the container's `agent`/`root` uid/gid onto your host user. This is great for file ownership, but it has one side effect for devices:

The serial node is owned `root:dialout` on the host. Inside the user namespace the group `dialout` (gid 20) **cannot be represented** — it is not a mapped subgid — so the node is remapped to `nobody:nogroup` and its group permission bits are **no longer honoured** for the container process.

When you inject a host node with `--device`, the runtime preserves the host node's permissions inside the container: `/dev/ttyACM0` is group-owned (`0660`) and the remap shows it as `nobody:nogroup`. Opening it requires the caller to be a member of the owning group. The container's **root** is mapped to a host sub-user, and its `CAP_DAC_OVERRIDE` is **not** honoured across that boundary — so the result is `EACCES` ("Permission denied") even though the process is root.

Running as the host user via `--userns=keep-id --user=<uid>:<gid>` sidesteps this: the uid/gid are preserved exactly, the device's group permission bits are honoured, and espbrew opens the node normally. It is the same mechanism the orchestrion agent container relies on — which is why that image bakes in `RUN usermod -aG nogroup agent` so the agent (uid 1000) is a member of `nogroup`.

> Adding your host user's groups with `--group-add=keep-groups` alone does **not** always help: if `dialout` is not a mapped subgid, the group membership cannot be carried into the namespace. The most reliable host-side fix is to make the node **world-accessible** (`MODE 0666`) — a deliberate, documented trade-off for a locally-connected USB debug unit, and the same approach used by many IoT/ESP flashing tools. See below.

## Host prerequisite: make the device world-accessible

Create a `udev` rule so the node is set to `0666` whenever it is plugged in. This is the most reliable way to let the container open the node, regardless of the container's user.

Create the file `/etc/udev/rules.d/69-espbrew-serial.rules`:

```
# Allow the container to open the ESP USB serial debug unit.
# Rootless user namespaces cannot represent the "dialout" group, so the node
# must be world-accessible for espbrew inside the container to open it.
SUBSYSTEM=="tty", ATTRS{idVendor}=="303a", MODE="0666", GROUP="dialout"
```

- `ATTRS{idVendor}=="303a"` matches Espressif devices. Adjust the vendor id if you use a different chip/FTDI/CP210x adapter.
- To match a specific board, you can additionally key on `ATTRS{idProduct}` or the serial string.

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

The rule re-applies automatically on every plug/reboot, so this is a one-time setup. If you do not have passwordless `sudo` or `udev`, you can alternatively `sudo chmod 0666 /dev/ttyACM0` before each launch, but the `udev` rule is the persistent fix.

> Note: `udev` is Linux-specific. On macOS and Windows the device node is accessible to the running user directly, so no rule is required.

### Host-side permissions (do not skip)

Two layers gate access to the device:

1. **Host layer** — the podman/docker user must be able to open the host `/dev/ttyACM0` node. On Linux that node is group-owned by `dialout` (or `users`).

   ```bash
   ls -la /dev/ttyACM0
   groups | grep -E "dialout|users"
   ```

2. **Container layer** — the injected node is reachable only by the mapped host user (`--userns=keep-id`) with the host's groups (`--group-add=keep-groups`).

If you are not in the owning group, add yourself and **log back in** (group membership is applied at login, not on the current shell):

```bash
sudo usermod -aG dialout "$USER"
# then log out / log back in (or `newgrp dialout` in a fresh shell)
```

This is documented in more detail in [`platform-support.md`](platform-support.md).

## State directory (HOME)

espbrew stores its state (database, bootloaders, captures) in `~/.espbrew`, resolved from `$HOME` (`os.UserHomeDir()`). The image sets `HOME=/`, and under `--userns=keep-id` the container process runs as your **host** user (not container root), so `/` is not writable and espbrew aborts:

```
create espbrew directory: mkdir /.espbrew: permission denied
```

Two fixes:

1. **Quick / throwaway:** pass `-e HOME=/tmp`. State is lost on restart.

2. **Persistent (recommended for a leader):** bind-mount a state dir owned by your user. Create it first, then mount it and point `HOME` at it:

   ```bash
   mkdir -p espbrew-state
   podman run --rm -p 8080:8080 --userns=keep-id --group-add=keep-groups \
     --device=/dev/ttyACM0:rwm \
     -v "$(pwd)/espbrew-state":/state -e HOME=/state \
     ghcr.io/georgik/espbrew-go:latest
   ```

> The image now pre-creates `/.espbrew` as a **world-writable** directory (see the `Containerfile`), so a bare run (`podman run … ghcr.io/georgik/espbrew-go:latest`) no longer fails with "permission denied" out of the box. That directory does **not** persist across container recreation, though — bind-mount a host dir as `HOME` (as above, or `-v ~/espbrew-state:/home/1000 -e HOME=/home/1000`, making sure the host dir is owned by the run uid) to keep device state persisted across restarts.

## Verifying the device reached the container

The cleanest check is the cluster start itself. Start the container exactly as in "Mounting a serial device" and watch stdout for the device watcher registering the board (path + serial MAC):

```
INF Device added path=/dev/ttyACM0 serial=30:30:F9:5A:8F:D4 vid=12346
```

If the board appears, the mount worked. If it reports "Permission denied" or the device is absent, the host-side `dialout` membership (host layer) or the keep-id/host-user flags (container layer) is the usual cause.

## Multiple devices

Mount as many nodes as your host exposes — add one `--device` flag per device (one `--group-add=keep-groups` covers them all):

```bash
podman run --rm \
  --userns=keep-id --user=1000:1000 --group-add=keep-groups \
  --device=/dev/ttyACM0:rwm \
  --device=/dev/ttyACM1:rwm \
  -p 8080:8080 \
  ghcr.io/georgik/espbrew-go:latest
```

## Mounting the stable `/dev/serial/by-id` path

For reproducibility you can mount the stable symlink instead of the volatile node name. espbrew resolves `/dev/serial/by-id/…` to the concrete `ttyACM*` node, so mounting either path works — mount the one that already exists on the host:

```bash
# Resolve your board's stable id first (example value)
readlink -f /dev/serial/by-id/usb-Espressif_ESP32-S3-DevKitC-1_1234567890-if00
# /dev/ttyACM0  -> then mount that node, as above
```

## Notes and troubleshooting

- **Device busy (`EBUSY`):** espbrew tolerates transient busy conditions — it briefly retries the port open when the boot-log probe is running or the USB device re-enumerates, so a flash succeeds without manual intervention. A *persistent* `Serial port busy` means a real second holder: `podman ps` to confirm no stray espbrew-go or shell container is running, kill it, and retry. Also confirm `ModemManager` on the host is not claiming the serial port. (Details in [flashing-implementation.md](flashing-implementation.md).)
- **Permission denied:** the device only opens when the container runs as the host user (`--userns=keep-id --user=<uid>:<gid>`). Running as the image default (root) yields `EACCES` because the remapped node is `nobody:nogroup` and root's capabilities do not cross the user-namespace boundary. Make the node world-accessible (`0666`) via the `udev` rule above, or confirm the keep-id flags + `dialout` membership.
- **`/.espbrew: permission denied`**: espbrew writes its config/state to `$HOME/.espbrew`. As uid 1000 with no `HOME` set it resolves to `/` (root-owned), which the process cannot write. Add `-e HOME=/tmp`, or bind-mount a writable host dir as `HOME` (e.g. `-v ~/espbrew-state:/home/1000 -e HOME=/home/1000`) to keep device state persisted across restarts — make sure the host dir is owned by uid 1000. (The image now pre-creates `/.espbrew` world-writable, so this is less common with current builds — see the State directory section.)
- **`--user=1000:1000`** must match the host user's uid/gid on the run host (run `id -u` / `id -g`). The keep-id mapping only works for a mapped uid.
- **`/dev/serial/by-id` not found**: a warning, not an error — espbrew falls back to a legacy `/dev/tty*` scan, which still discovers the board.
