## Simulator Backends

ESPBrew supports simulator backends for testing without physical hardware. This enables testing and development workflows without requiring actual ESP32 devices.

### Supported Backends

**Wokwi Simulator**
- ESP32, ESP32-S2, ESP32-S3, ESP32-C3, ESP32-C6 support
- Diagram-based circuit simulation
- Serial output monitoring
- Firmware flashing simulation

**QEMU (Planned)**
- Full system emulation
- Debugging support

### Creating Virtual Devices

Create virtual devices via the REST API:

```bash
# Create a Wokwi device
curl -X POST http://localhost:8080/api/v1/devices/virtual \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "wokwi-esp32-test",
    "chip_type": "ESP32",
    "description": "Test Wokwi device",
    "backend": "wokwi",
    "backend_config": {
      "diagram_json": "{\"version\":1,\"parts\":[{\"type\":\"esp32-devkitC\",\"id\":\"chip\"}]}"
    }
  }'
```

### Using Virtual Devices

Virtual devices work with the same commands as physical devices:

```bash
# Flash to virtual device
./espbrew flash firmware.bin -d wokwi-esp32-test

# Monitor virtual device
./espbrew monitor -d wokwi-esp32-test

# Snap with virtual device
./espbrew snap firmware.bin -d wokwi-esp32-test
```

### Backend Configuration

Set backend configuration for existing devices:

```bash
# Configure a device as Wokwi simulator
curl -X PUT http://localhost:8080/api/v1/devices/{id}/backend \
  -H "Content-Type: application/json" \
  -d '{
    "backend": "wokwi",
    "backend_config": {
      "chip_type": "ESP32",
      "diagram_json": "{\"version\":1,\"parts\":[{\"type\":\"esp32-devkitC\",\"id\":\"chip\"}]}"
    }
  }'
```

### Requirements

**Wokwi Simulator:**
- Install wokwi-cli: `npm install -g wokwi-cli`
- Valid diagram.json for your chip type
- ELF firmware file
