# CI/CD Setup Summary

## Overview

ESPBrew-Go now has CI/CD configured on both Codeberg (primary) and GitHub (mirror).

## Platforms Configured

### Codeberg (Primary Repository)
- **Platform**: Woodpecker CI
- **Location**: `.woodpecker/`
- **Pipelines**:
  - `test.yml` - Runs on push/PR to main/develop
  - `release.yml` - Runs on version tags (v*)

### GitHub (Mirror Repository)
- **Platform**: GitHub Actions
- **Location**: `.github/workflows/`
- **Workflows**:
  - `ci.yml` - Tests on Linux, macOS, Windows
  - `release.yml` - Multi-platform builds on tags

## Configuration Files

```
.woodpecker/
├── test.yml          # Codeberg test pipeline
└── release.yml       # Codeberg release pipeline

.github/
├── workflows/
│   ├── ci.yml        # GitHub Actions CI
│   └── release.yml   # GitHub Actions release
└── dependabot.yml    # Dependency updates

scripts/
├── build-release.sh   # Manual release build script
└── create-release.sh  # Release creation helper
```

## Build Limitations

### V4L2 Camera Support
The `github.com/vladimirvivien/go4vl` library requires CGO and Linux-specific headers. This affects cross-compilation:

- **Linux amd64**: Full camera support (CGO enabled)
- **Linux arm64/arm**: No camera support (CGO disabled)
- **macOS/Windows**: No camera support (platform-specific)

### Build Matrix

| Platform | Architecture | Camera Support | Built By |
|----------|-------------|----------------|----------|
| Linux | amd64 | Full | Codeberg + GitHub |
| Linux | arm64 | None | GitHub |
| Linux | arm | None | GitHub |
| macOS | amd64 | None | GitHub |
| macOS | arm64 | None | GitHub |
| Windows | amd64 | None | GitHub |

## Release Process

### Option 1: Using GitHub Actions (Recommended)
1. Push tag to GitHub: `git push gh v1.0.0`
2. GitHub Actions builds all platforms
3. Release created automatically with checksums

### Option 2: Using Codeberg CI
1. Push tag to Codeberg: `git push origin v1.0.0`
2. Woodpecker builds Linux binaries
3. Release created on Codeberg

### Option 3: Manual Build
```bash
# Build for current platform
./scripts/build-release.sh

# Create and push release tag
./scripts/create-release.sh v1.0.0 "Release v1.0.0"
```

## CI Pipeline Details

### Codeberg Test Pipeline (`.woodpecker/test.yml`)
- Runs on: Pull requests, pushes to main/develop
- Steps:
  - Format check
  - Go vet
  - Tests with race detector
  - WASM build verification
  - golangci-lint

### Codeberg Release Pipeline (`.woodpecker/release.yml`)
- Runs on: Version tags (v*)
- Builds:
  - Linux amd64 (with CGO)
  - Linux arm64/arm (without CGO)
  - WASM UI
- Creates release on Codeberg with SHA256 checksums

### GitHub Actions CI (`.github/workflows/ci.yml`)
- Runs on: Pull requests, pushes to main/develop
- Tests on: Linux, macOS, Windows
- Includes: WASM size check

### GitHub Actions Release (`.github/workflows/release.yml`)
- Runs on: Version tags (v*)
- Builds all platforms in parallel
- Creates GitHub release with:
  - All binaries
  - SHA256 checksums
  - Auto-generated release notes

## Enabling CI

### Codeberg Woodpecker CI
1. Go to: https://codeberg.org/georgik/espbrew-go/settings
2. Enable "Woodpecker CI"
3. Configure webhook if needed

### GitHub Actions
1. Already enabled on GitHub mirror
2. No configuration needed

## Dependency Updates

Dependabot is configured to:
- Check weekly (Mondays)
- Update Go modules
- Update GitHub Actions
- Create PRs with `dependencies` and `go` labels

## Testing CI

### Test Codeberg CI
```bash
# Create a test branch
git checkout -b test/ci-setup

# Make a trivial change
echo "# CI test" >> README.md

# Commit and push
git add README.md
git commit -m "test: CI setup verification"
git push origin test/ci-setup

# Create PR on Codeberg and check Woodpecker status
```

### Test GitHub CI
```bash
# Push to GitHub mirror
git push gh test/ci-setup

# Create PR on GitHub and check Actions status
```

### Test Release
```bash
# Create a test tag (will be deleted later)
git tag v0.0.0-test
git push origin v0.0.0-test
git push gh v0.0.0-test

# Check releases on both platforms
# Codeberg: https://codeberg.org/georgik/espbrew-go/releases
# GitHub: https://github.com/georgik/espbrew-go/releases

# Delete test tag
git tag -d v0.0.0-test
git push origin :refs/tags/v0.0.0-test
git push gh :refs/tags/v0.0.0-test
```

## Next Steps

1. Enable Woodpecker CI in Codeberg repository settings
2. Test with a dummy PR/commit
3. Test release flow with test tag
4. Add CI status badges to README

## Badge Markdown

```markdown
[![CI](https://ci.codeberg.org/api/badges/georgik/espbrew-go/status.svg)](https://ci.codeberg.org/georgik/espbrew-go)
[![GitHub CI](https://github.com/georgik/espbrew-go/actions/workflows/ci.yml/badge.svg)](https://github.com/georgik/espbrew-go/actions/workflows/ci.yml)
```
