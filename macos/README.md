# macOS Camera Permission Configuration

This directory contains macOS-specific configuration files for camera access.

## Files

- `Info.plist` - macOS application metadata (not used for CLI, kept for reference)
- `espbrew.entitlements` - Entitlements for camera, USB, and network access

## Camera Permissions on macOS

macOS requires code signing for camera access. This applies even to ad-hoc signed binaries.

### Local Development

Use the macOS-specific build target:

```bash
make macos-build
```

This applies ad-hoc signing with the entitlements. On first run, macOS will prompt for camera permission in System Preferences.

### Verifying Signing

Check if the binary is properly signed:

```bash
codesign -d -vv ./espbrew
codesign --display --entitlements - ./espbrew
```

### First Run - Granting Permissions

When espbrew first tries to access the camera:

1. macOS will show a permission dialog
2. Grant camera access when prompted
3. Or manually enable in: System Settings → Privacy & Security → Camera

### Official Release Signing

For production releases, sign with your Apple Developer certificate:

```bash
# Build the binary
go build -o espbrew ./cmd/espbrew

# Sign with your certificate
codesign --sign "Developer ID Application: Your Name" \
         --entitlements macos/espbrew.entitlements \
         --deep ./espbrew

# Verify
codesign -d -vv ./espbrew
spctl -a -t execute -vv ./espbrew
```

### Required Entitlements

- `com.apple.security.device.camera` - Camera access for display photography
- `com.apple.security.device.microphone` - Required by AVFoundation (even if unused)
- `com.apple.security.device.usb` - USB device access for ESP32 flashing
- `com.apple.security.network.client/server` - Cluster communication
- `com.apple.security.files.user-selected.read-write` - File access for firmware images

## Troubleshooting

### Camera Not Detected

1. Check if binary is signed:
   ```bash
   codesign -d -vv ./espbrew
   ```

2. Verify camera permission in System Settings

3. Check logs for permission errors:
   ```bash
   ./espbrew cluster --role leader --port 8080
   ```

### Code Signing Errors

If codesign fails, ensure Xcode Command Line Tools are installed:

```bash
xcode-select --install
```

