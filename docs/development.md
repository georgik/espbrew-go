## Development

```bash
# Initialize submodules (required for build)
git submodule update --init --recursive

# Update submodules to latest versions
git submodule update --remote

# Format code
gofmt -w .

# Run linter
go vet ./...

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Build all
go build ./...

# Run E2E tests (requires hardware)
make e2e

# Run E2E tests in short mode (skip flash)
make e2e-short
```

### Linting

The project uses [golangci-lint](https://golangci-lint.run/) for code quality checks. CI runs linter automatically on all pull requests.

**Install golangci-lint:**
```bash
# Via install script (recommended)
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Or via package manager
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

**Run linter locally:**
```bash
# Basic run
golangci-lint run

# Run with specific config
golangci-lint run --config .golangci.yml

# Run only specific linters
golangci-lint run --disable-all --enable errcheck,govet

# Fix issues automatically
golangci-lint run --fix
```

**Common Issues and Fixes:**

1. **errcheck (unchecked error returns)**
   ```go
   // Bad: defer port.Close()
   // Good:
   defer func() { _ = port.Close() }()
   
   // Bad: _ = fmt.Scanf(...)
   // Good (scanf returns 2 values):
   _, _ = fmt.Scanf(...)
   
   // Bad: conn.WriteMessage(...)
   // Good:
   _ = conn.WriteMessage(...)
   ```

2. **govet (structural issues)**
   ```go
   // Bad: if v.Kind() == reflect.Ptr
   // Good (inline constant):
   const ptrKind = reflect.Ptr
   if v.Kind() == ptrKind
   ```

3. **ineffassign (ineffectual assignments)**
   ```go
   // Bad: assignment before return
   if oldState != nil {
       term.Restore(int(os.Stdin.Fd()), oldState)
       oldState = nil  // Useless, function returns
   }
   return nil
   
   // Good: remove useless assignment
   if oldState != nil {
       term.Restore(int(os.Stdin.Fd()), oldState)
   }
   return nil
   ```

**CI Configuration:** `.github/workflows/ci.yml` runs linter with latest version.

### End-to-End Tests

E2E tests validate the complete snap workflow against a real cluster server with actual hardware:

```bash
# Via Makefile (recommended)
make e2e              # Full E2E test with flash
make e2e-short        # Skip flash, faster testing

# Via go test
go test -tags=e2e -v -run TestE2E_SnapWithCluster ./cmd/espbrew
go test -tags=e2e -short -v ./cmd/espbrew
```

**Prerequisites:**
- ESP32-S3-Box-3 device connected via USB
- Project firmware built (from test project or your own)
- Device available at `/dev/ttyACM0` or `/dev/ttyUSB0`

**Test Coverage:**
- Server startup with dev mode enabled
- Health check endpoint validation
- Device discovery and selection
- Snap with skip-flash (5s duration)
- Optional snap with flash (full workflow)
- Response validation (snap_id, status, duration)
- Server shutdown via API

**Test Project:** By default uses `/home/georgik/projects/esp32-conways-game-of-life-rs/esp32-s3-box-3`. Modify `e2eProjectDir` in `cmd/espbrew/snap_e2e_test.go` for your setup.

### Stub Loaders

ESP stub loaders are tracked via git submodule from esp-rs/espflash repository. The stubs enable advanced flashing features (MD5 verification, compressed flashing, region erase).

**Update stubs from upstream:**
```bash
# Checkout specific version in submodule
cd vendor/espflash && git checkout v3.1.0 && cd ../..

# Copy stubs to local directory
go run tools/update-stubs.go
```

**Stub location:** `internal/espflash/stubs/` (embedded in binary via go:embed)

### Test Data

Some integration tests require ESP32 ELF files. See [internal/flash/testdata/README.md](internal/flash/testdata/README.md) for setup instructions.

Tests requiring external data will skip gracefully if not available. To run all tests:

```bash
export ESPBREW_TEST_ELF="/path/to/your/esp32s3-binary"
go test ./internal/flash/... -v
```

