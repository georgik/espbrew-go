# ESPBrew Makefile

.PHONY: all build wasm clean test fmt vet lint run e2e demo demo-serve

# Default target builds everything
all: build

# Build both WASM UI and server binary
build: wasm
	@echo "Building ESPBrew server..."
	@go build -o espbrew ./cmd/espbrew
	@echo "Build complete: ./espbrew"
	@echo "WASM size: $$(wc -c < web/main.wasm) bytes"

# Build WASM UI only
wasm:
	@echo "Building WASM UI..."
	@GOOS=js GOARCH=wasm go build -o web/main.wasm ./cmd/wasm
	@echo "WASM built: web/main.wasm ($$(wc -c < web/main.wasm) bytes)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f espbrew
	@rm -f web/main.wasm
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	@go test -v -race -count=1 ./...

# Run E2E tests (requires hardware device)
e2e:
	@echo "Running E2E tests..."
	@go test -tags=e2e -v -count=1 ./cmd/espbrew

# Run E2E tests in short mode (skip flash)
e2e-short:
	@echo "Running E2E tests (short mode)..."
	@go test -tags=e2e -short -v -count=1 ./cmd/espbrew

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Run linter (builds from source for Go 1.26+ compatibility)
lint:
	@echo "Building golangci-lint from source for Go 1.26 compatibility..."
	@go run github.com/golangci/golangci-lint/cmd/golangci-lint@master run ./... 2>/dev/null || \
	(echo "Falling back to basic checks..." && go vet ./... && go fmt ./...)

# Run server (default port 8080)
run: build
	@echo "Starting ESPBrew server on http://localhost:8080"
	@./espbrew serve

# Run with custom port
run-port:
	@echo "Starting ESPBrew server on http://localhost:$(PORT)"
	@./espbrew serve --port $(PORT)

# Install to $GOPATH/bin
install: wasm
	@echo "Installing ESPBrew..."
	@go install ./cmd/espbrew
	@echo "Installed: $$(which espweb)"

# Development: build and run with file watching (requires air)
dev:
	@echo "Starting development server with hot reload..."
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "air not installed. Run: go install github.com/cosmtrek/air@latest"; \
		$(MAKE) run; \
	fi

# Build demo package for GitHub Pages or static hosting
demo: wasm
	@echo "Creating demo package..."
	@rm -rf demo
	@mkdir -p demo/v2
	@cp web/index.html demo/v2/
	@cp web/styles.css demo/v2/
	@cp web/wasm_exec.js demo/v2/
	@cp web/main.wasm demo/v2/
	@cp -r web/static demo/v2/
	@cp web/demo/index.html demo/
	@echo "Demo package ready: demo/"
	@echo "  Size: $$(du -sh demo | cut -f1)"
	@echo ""
	@echo "To test locally:"
	@echo "  make demo-serve"
	@echo ""
	@echo "Demo URL with demo mode enabled:"
	@echo "  https://georgik.github.io/espbrew-go/v2/?demo=true"

# Serve demo package locally for testing
demo-serve: demo
	@echo "Starting demo server on http://localhost:8000"
	@echo "Press Ctrl+C to stop"
	@cd demo && python3 -m http.server 8000

# Show help
help:
	@echo "ESPBrew Build Commands:"
	@echo ""
	@echo "  make              - Build everything (WASM + server)"
	@echo "  make build        - Same as above"
	@echo "  make wasm         - Build WASM UI only"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make test         - Run tests"
	@echo "  make e2e          - Run E2E tests (requires hardware)"
	@echo "  make e2e-short    - Run E2E tests short mode (no flash)"
	@echo "  make fmt          - Format code"
	@echo "  make vet          - Run go vet"
	@echo "  make lint         - Run golangci-lint"
	@echo "  make run          - Build and run server"
	@echo "  make run-port PORT=3000  - Run on custom port"
	@echo "  make install      - Install to GOPATH/bin"
	@echo "  make dev          - Run with hot reload (requires air)"
	@echo ""
	@echo "Demo (GitHub Pages / static hosting):"
	@echo "  make demo         - Build demo package for static hosting"
	@echo "  make demo-serve   - Serve demo locally on port 8000"
	@echo ""
	@echo "Access:"
	@echo "  http://localhost:8080/  - Legacy HTML UI"
	@echo "  http://localhost:8080/v2/ - WASM UI"
	@echo "  http://localhost:8080/v2/?demo=true - WASM UI demo mode"
