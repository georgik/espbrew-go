#!/usr/bin/env bash
# Build ESPBrew release packages for multiple platforms

set -euo pipefail

VERSION="${VERSION:-dev}"
OUTPUT_DIR="${OUTPUT_DIR:-release}"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Platforms to build for
# Note: Due to V4L2 CGO constraints, we only build Linux amd64 from Linux host
# Other platforms require native builds (handled by GitHub Actions)
declare -A PLATFORMS=()

# Detect native platform
NATIVE_OS="$(go env GOOS)"
NATIVE_ARCH="$(go env GOARCH)"

# Always build native platform
EXT=""
if [[ "${NATIVE_OS}" == "windows" ]]; then
    EXT=".exe"
fi
PLATFORMS["${NATIVE_OS}-${NATIVE_ARCH}"]="${NATIVE_OS} ${NATIVE_ARCH} espbrew-${NATIVE_OS}-${NATIVE_ARCH}${EXT}"

# For Linux amd64 hosts, add other Linux architectures
if [[ "${NATIVE_OS}" == "linux" && "${NATIVE_ARCH}" == "amd64" ]]; then
    log_warn "Cross-compilation limited - V4L2 library requires amd64"
    log_warn "Other architectures built without camera support"
fi

# Create output directory
mkdir -p "${OUTPUT_DIR}"

# Build for each platform
for platform in "${!PLATFORMS[@]}"; do
    IFS=' ' read -r goos goarch output <<< "${PLATFORMS[$platform]}"
    log_info "Building ${platform}..."

    # Enable CGO only for native platform
    cgo_enabled="0"
    if [[ "${goos}" == "${NATIVE_OS}" && "${goarch}" == "${NATIVE_ARCH}" ]]; then
        cgo_enabled="1"
    fi

    GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED="${cgo_enabled}" \
        go build -ldflags="${LDFLAGS}" -o "${OUTPUT_DIR}/${output}" ./cmd/espbrew

    # Make executable (except Windows)
    if [[ "${output}" != *.exe ]]; then
        chmod +x "${OUTPUT_DIR}/${output}"
    fi

    # Show size
    SIZE=$(wc -c < "${OUTPUT_DIR}/${output}")
    log_info "Built ${output} (${SIZE} bytes)"
done

# Build WASM
log_info "Building WASM..."
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o "${OUTPUT_DIR}/espbrew-wasm.wasm" ./cmd/wasm
WASM_SIZE=$(wc -c < "${OUTPUT_DIR}/espbrew-wasm.wasm")
log_info "Built espbrew-wasm.wasm (${WASM_SIZE} bytes)"

# Generate checksums
log_info "Generating SHA256 checksums..."
cd "${OUTPUT_DIR}"
sha256sum * > SHA256SUMS
cd ..

log_info "Release packages built successfully in ${OUTPUT_DIR}/"
log_info "Checksum file: ${OUTPUT_DIR}/SHA256SUMS"

# Show summary
echo ""
echo "Build Summary:"
echo "============="
for file in "${OUTPUT_DIR}"/*; do
    if [ -f "$file" ]; then
        SIZE=$(wc -c < "$file")
        echo "  $(basename "$file"): ${SIZE} bytes"
    fi
done
