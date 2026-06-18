#!/usr/bin/env bash
# Create a new release with proper tagging and artifact generation

set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if version is provided
if [ $# -eq 0 ]; then
    log_error "Usage: $0 <version> [message]"
    echo "Example: $0 v1.0.0 \"Release v1.0.0\""
    exit 1
fi

VERSION="$1"
MESSAGE="${2:-Release ${VERSION}}"

# Validate version format (vX.Y.Z)
if [[ ! "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
    log_error "Invalid version format: ${VERSION}"
    echo "Version must follow semantic versioning: vX.Y.Z or vX.Y.Z-<pre-release>"
    exit 1
fi

# Check if tag already exists
if git rev-parse "${VERSION}" >/dev/null 2>&1; then
    log_error "Tag ${VERSION} already exists"
    echo "Use 'git tag -d ${VERSION}' to delete it locally if you want to recreate"
    exit 1
fi

# Check if working directory is clean
if [ -n "$(git status --porcelain)" ]; then
    log_warn "Working directory has uncommitted changes"
    echo "Commit or stash changes before creating a release"
    exit 1
fi

log_info "Creating release ${VERSION}..."
echo ""

# Build release packages
log_info "Building release packages..."
VERSION="${VERSION}" ./scripts/build-release.sh

echo ""

# Show what will be created
log_info "Will create:"
echo "  - Git tag: ${VERSION}"
echo "  - Tag message: ${MESSAGE}"
echo ""
echo "Release artifacts:"
ls -la release/

echo ""
read -p "Continue? (y/N) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Release cancelled"
    exit 0
fi

# Create and push tag
log_info "Creating tag ${VERSION}..."
git tag -a "${VERSION}" -m "${MESSAGE}"

log_info "Pushing tag to origin (Codeberg)..."
git push origin "${VERSION}"

log_info "Pushing tag to gh (GitHub mirror)..."
git push gh "${VERSION}" || log_warn "Failed to push to GitHub (may not be configured)"

echo ""
log_info "Release ${VERSION} created successfully!"
echo ""
echo "Next steps:"
echo "  1. CI will build and publish artifacts automatically"
echo "  2. Check Codeberg: https://codeberg.org/georgik/espbrew-go/releases"
echo "  3. Check GitHub: https://github.com/georgik/espbrew-go/releases"
