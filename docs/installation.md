## Installation

```bash
# Clone repository
git clone https://github.com/georgik/espbrew-go.git
cd espbrew-go

# Initialize submodules
git submodule update --init --recursive

# Build the application
go build -o espbrew ./cmd/espbrew

# Linux/macOS: Install to system path
sudo mv espbrew /usr/local/bin/

# Windows: Add espbrew.exe to your PATH or use from current directory
```

## Environment Variables

ESPBrew supports environment variables for configuration without command-line flags:

### Cluster Configuration

- **ESPBREW_CLUSTER**: Default cluster URL for remote operations
- **ESPBREW_LEADER**: Default leader address for cluster mode

```bash
# Set default cluster URL
export ESPBREW_CLUSTER=http://leader:8080

# All cluster commands now use the default
espbrew flash firmware.bin              # Uses ESPBREW_CLUSTER
espbrew monitor                         # Uses ESPBREW_CLUSTER
espbrew snap                             # Uses ESPBREW_CLUSTER

# Override with --cluster flag when needed
espbrew --cluster http://other:8080 flash firmware.bin
```

### Leader Node Configuration

```bash
# Set default leader for peer nodes
export ESPBREW_LEADER=leader:8080

# Start peer with default leader
espbrew cluster --role peer --node-id station-1
```

### Usage Examples

```bash
# Daily workflow with environment variables
export ESPBREW_CLUSTER=http://build-server:8080
idf.py build
espbrew flash                          # Auto-build, auto-flash to cluster
espbrew snap --duration 5              # Quick verification
```

```
