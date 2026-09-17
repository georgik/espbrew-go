# CI/CD Setup Summary

ESPBrew-Go runs **GitHub Actions** only (Woodpecker/Codeberg was a removed experiment).
Everything — tests, multi-platform release builds, and the container image — lives under
`.github/workflows/`.

## Workflows (`.github/workflows/`)

| File | Trigger | What it does |
|------|---------|--------------|
| `ci.yml` | push to `main`/`develop`, PRs | Tests on Linux/macOS/Windows, WASM build + size check, lint (`go vet`) |
| `release.yml` | version tags `v*` | Builds all platforms in parallel (linux amd64/arm64, darwin arm64, windows), uploads artifacts, creates the GitHub Release with SHA256 checksums |
| `publish-image.yml` | **manual** (`workflow_dispatch`) | Bundles the pre-built release binary into a container image and pushes it to `ghcr.io` |
| `demo.yml` | **manual** (`workflow_dispatch`) | Builds the WASM demo and deploys it to GitHub Pages |
| `dependabot.yml` | — | Weekly dependency updates (Go modules + GitHub Actions) |

## Configuration Files

```
.github/
├── workflows/
│   ├── ci.yml            # GitHub Actions CI (tests + lint)
│   ├── release.yml       # GitHub Actions release (multi-platform binaries)
│   ├── publish-image.yml # Container image -> ghcr.io (manual)
│   └── demo.yml          # WASM demo -> GitHub Pages (manual)
└── dependabot.yml        # Dependency updates

scripts/
├── build-release.sh      # Manual release build script
└── create-release.sh     # Release creation helper
```

## Build Limitations

### V4L2 Camera Support
The `github.com/vladimirvivien/go4vl` library requires CGO and Linux-specific headers. This affects
cross-compilation:

- **Linux amd64**: Full camera support (CGO enabled)
- **Linux arm64/arm**: No camera support (CGO disabled)
- **macOS/Windows**: No camera support (platform-specific)

### Build Matrix

| Platform | Architecture | Camera Support | Built By |
|----------|--------------|----------------|----------|
| Linux | amd64 | Full | GitHub (ubuntu-latest) |
| Linux | arm64 | None | GitHub (ubuntu-24.04-arm, free native runner) |
| Linux | arm | None | GitHub |
| macOS | amd64 | None | GitHub |
| macOS | arm64 | None | GitHub |
| Windows | amd64 | None | GitHub |

> The native `ubuntu-24.04-arm` runner is required for `linux/arm64` because the Linux build pulls in
> `go4vl` (CGO + `linux/videodev2.h`); plain cross-compilation from amd64 fails without an aarch64
> toolchain.

## Release Process

### Option 1: GitHub Actions (Recommended)
1. Push a tag to GitHub: `git tag v1.0.0 && git push origin v1.0.0`
2. The `release.yml` workflow builds all platforms
3. The GitHub Release is created automatically with all binaries + checksums

### Option 2: Manual Build
```bash
# Build for current platform (+ other Linux archs if on linux/amd64)
./scripts/build-release.sh

# Create and push release tag
./scripts/create-release.sh v1.0.0 "Release v1.0.0"
```

## Pipeline Details

### CI (`.github/workflows/ci.yml`)
- Runs on: Pull requests, pushes to `main`/`develop`
- Tests on: Linux, macOS, Windows (with `-race`)
- WASM build + 5 MB size warning
- Lint: `go vet` (golangci-lint is pending Go 1.26 support)

### Release (`.github/workflows/release.yml`)
- Runs on: Version tags (`v*`)
- Builds all platforms in parallel via a matrix
- Creates the GitHub Release with:
  - All binaries
  - SHA256 checksums
  - Auto-generated release notes

### Container Image — `ghcr.io` (`.github/workflows/publish-image.yml`)

A container image that runs espbrew as a **cluster leader** (web dashboard + API on port **8080**)
is published to the **GitHub Container Registry** (`ghcr.io`) — no DockerHub account needed, and the
image lives next to the code that produced it.

- **`Containerfile`** (repo root) — bundles the **pre-built release binary** `espbrew-linux-amd64`
  (downloaded from GitHub Releases, **no build inside the image**) and starts it with
  `espbrew cluster --role leader --port 8080` (mirrors `cluster.sh`). Base image is `debian:bookworm-slim`
  because the release binary is glibc-linked (Alpine/musl would not run it).
- **`.github/workflows/publish-image.yml`** — triggered **manually** (`workflow_dispatch`). Bundles a
  configurable release tag (default `v0.3.1`), builds the image with `docker/build-push-action`, and
  pushes it to `ghcr.io/<owner>/<repo>`.

#### Use

1. Go to `<repo> -> Actions -> Publish Image -> Run workflow`.
2. Set **espbrew release tag to bundle** (e.g. `v0.3.1`) and optionally an extra image tag.
3. After it finishes, pull and run:

   ```bash
   docker login ghcr.io            # your GitHub credentials
   docker pull ghcr.io/<owner>/<repo>:latest
   docker run --rm -p 8080:8080 ghcr.io/<owner>/<repo>:latest
   # -> espbrew cluster running, dashboard at http://localhost:8080
   ```

   To bundle a different release, change the **version** input (or rebuild with
   `docker build --build-arg ESPBREW_VERSION=vX.Y.Z .`).

## Dependency Updates

Dependabot is configured to:
- Check weekly (Mondays)
- Update Go modules
- Update GitHub Actions
- Create PRs with `dependencies` and `go` labels

## Testing CI

```bash
# Create a test branch, make a trivial change, commit and push
git checkout -b test/ci-setup
echo "# CI test" >> README.md
git add README.md
git commit -m "test: CI setup verification"
git push origin test/ci-setup

# Open a PR on GitHub and check the Actions status
```

### Test Release

```bash
# Create a test tag (deleted afterwards)
git tag v0.0.0-test
git push origin v0.0.0-test

# Check the GitHub Release that gets created
# https://github.com/georgik/espbrew-go/releases

# Delete the test tag
git tag -d v0.0.0-test
git push origin :refs/tags/v0.0.0-test
```

## Status Badges

```markdown
[![GitHub CI](https://github.com/georgik/espbrew-go/actions/workflows/ci.yml/badge.svg)](https://github.com/georgik/espbrew-go/actions/workflows/ci.yml)
[![Release](https://github.com/georgik/espbrew-go/actions/workflows/release.yml/badge.svg)](https://github.com/georgik/espbrew-go/actions/workflows/release.yml)
[![Publish Image](https://github.com/georgik/espbrew-go/actions/workflows/publish-image.yml/badge.svg)](https://github.com/georgik/espbrew-go/actions/workflows/publish-image.yml)
```
