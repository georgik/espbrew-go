## Hardware Support

Supports all ESP32 variants with chip-specific bootloader offsets and automatic bootloader management. ESP32-S3, ESP32-C3, ESP32-C5, ESP32-C6, and ESP32-H2 chips with USB-JTAG/Serial support are automatically detected and use the appropriate reset method.

| Chip      | Bootloader Offset | Bootloader Size | Notes                   |
|-----------|-------------------|-----------------|-------------------------|
| ESP32     | 0x1000            | ~26 KB          | Original ESP32 only     |
| ESP32-S2  | 0x1000            | ~22 KB          |                         |
| ESP32-S3  | 0x0               | ~21 KB          |                         |
| ESP32-C2  | 0x0               | ~20 KB          |                         |
| ESP32-C3  | 0x0               | ~21 KB          |                         |
| ESP32-C5  | 0x2000            | ~22 KB          |                         |
| ESP32-C6  | 0x0               | ~23 KB          |                         |
| ESP32-H2  | 0x0               | ~22 KB          |                         |
| ESP32-C61 | 0x0               | ~22 KB          |                         |
| ESP32-P4  | 0x2000            | ~23 KB          | Rev 0, Rev 1 supported   |

Bootloaders are automatically downloaded from the espflash project repository on first use and cached locally for subsequent operations.

## File Type Detection

ESPBrew automatically detects and processes firmware file types:

- **ELF files**: Automatically converted to ESP-IDF format with bootloader and partition table
- **ESP32 Binary**: Magic 0xE9 (ESP32) or 0xEA (ESP8266)
- **Raw Binary**: Flashed as-is to specified offset

### ELF File Support

When flashing ELF files (Rust no_std or TinyGo ESP projects), ESPBrew:

1. Extracts ROM and RAM segments from the ELF
2. Downloads appropriate bootloader for the target chip
3. Generates default partition table
4. Creates ESP-IDF format image with proper checksums
5. Flashes the complete image

This enables direct flashing of Rust ESP and TinyGo projects without intermediate conversion steps.

### Technical Implementation Notes

ELF to ESP image conversion uses ESP-IDF ExtendedImageHeader format (24 bytes header):

- **Flash Size Encoding**: Byte 3 encodes both size and frequency as `(size_enum << 4) | freq_enum`
  - Size enum: 0x00=1MB, 0x01=2MB, 0x02=4MB, 0x04=16MB, etc.
  - Frequency enum: 0x00=40MHz, 0x01=26MHz, 0x02=20MHz, 0x0F=80MHz
  - Example: 16MB @ 40MHz = 0x40 (0x04 << 4 | 0x00)

- **Segment Merging**: ROM segments from adjacent memory regions are merged with proper padding
  - IROM segment starts at 0x42000000 (ESP32-S3 code flash)
  - DROM segment starts at 0x3C000000 (ESP32-S3 data flash)
  - Segments are sorted by address and merged with zero padding for gaps

- **Image Structure**: App image follows ESP-IDF v3.0 format
  - 24-byte extended header with chip ID, entry point, WP pin
  - Segment headers (8 bytes each): address + length
  - Segment data followed by SHA-256 digest

## Project Detection

ESPBrew automatically detects project types when run from a project directory. This eliminates the need to manually specify paths to bootloader, partition table, and application binaries.

### ESP-IDF Projects

Detection requires:
- `CMakeLists.txt` in project root
- `sdkconfig` or `sdkconfig.defaults` file
- `build/` directory with compiled binaries

When detected, ESPBrew automatically locates:
- `build/bootloader/bootloader.bin`
- `build/partition_table/partition-table.bin`
- `build/<project_name>.bin` (or largest `.bin` file)

```bash
cd esp-idf-project
idf.py build
espbrew --cluster http://leader:8080 flash
# Output: Detected ESP-IDF project, auto-populated flash paths
```

### TinyGo Projects

Detection requires:
- `go.mod` in project root
- `tinygo.org/x/*` dependencies (e.g., `tinygo.org/x/drivers`)
- OR source files importing `"machine"` package

When detected, ESPBrew automatically:
- Converts ELF output to ESP-IDF format with bootloader
- Injects partition table
- Flashes complete image to device

```bash
cd tinygo-project
tinygo build -target=esp32s3-box-3 .
espbrew --cluster http://leader:8080 flash
# Output: Detected tinygo project, auto-populated flash paths
```

**Supported TinyGo targets:** ESP32, ESP32-S2, ESP32-S3, ESP32-C3, ESP32-C6, ESP32-H2

### Rust no_std Projects

Detection requires:
- `Cargo.toml` in project root
- ESP HAL dependencies (e.g., `esp-hal`, `esp-backtrace`)
- `.cargo/config.toml` with ESP target triple

When detected, ESPBrew automatically:
- Converts ELF output to ESP-IDF format with bootloader
- Injects partition table
- Flashes complete image to device

```bash
cd rust-esp-project
cargo build --release
espbrew --cluster http://leader:8080 flash
# Output: Detected rust-esp project, auto-populated flash paths
```

### Disabling Auto-Detection

To disable automatic project detection:

```bash
espbrew flash --no-detect
```

### Explicit Override

Explicitly specified paths always override auto-detected paths:

```bash
espbrew flash --app custom.bin --partitions custom-partitions.bin
```

## ESP-IDF Integration

Use `--build-dir` to flash ESP-IDF projects:

```bash
cd esp-idf-project
idf.py build
espbrew flash --build-dir build/
```

Reads `build/flash_args` for flash settings and file list. Automatically finds files in:
- `{build-dir}/{filename}`
- `{build-dir}/bootloader/{filename}`
- `{filename}` (fallback)

## Bootloader Management

ESPBrew automatically downloads and caches ESP32 bootloaders from the official espflash repository on first use. Bootloaders are stored in `~/.espbrew/bootloaders/` and reused for subsequent flashes.

### Supported Chips

Bootloaders are downloaded from espflash v3.1.0 for:

ESP32, ESP32-S2, ESP32-S3, ESP32-C2, ESP32-C3, ESP32-C5, ESP32-C6, ESP32-C61, ESP32-H2, ESP32-P4

### Custom Bootloaders

To use a custom bootloader binary:

```bash
espbrew flash --bootloader /path/to/custom-bootloader.bin firmware.bin
```

### Cache Management

The bootloader cache is automatically managed:

- First use: Downloads required bootloader (~21 KB per chip)
- Subsequent uses: Loads from cache (no network access)
- Cache location: `~/.espbrew/bootloaders/`

To clear the cache:

```bash
rm -rf ~/.espbrew/bootloaders/
```

### Offline Operation

Once bootloaders are cached, ESPBrew works offline. For air-gapped environments, pre-populate the cache by running espbrew once with network access, or manually copy bootloader binaries to `~/.espbrew/bootloaders/`.

