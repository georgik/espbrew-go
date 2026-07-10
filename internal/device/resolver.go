package device

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// DeviceResolver resolves stable device identifiers and extracts device attributes
type DeviceResolver struct {
	mu    sync.RWMutex
	cache map[string]*DeviceInfo // by-id -> DeviceInfo
}

// NewDeviceResolver creates a new device resolver
func NewDeviceResolver() *DeviceResolver {
	return &DeviceResolver{
		cache: make(map[string]*DeviceInfo),
	}
}

// ResolvePath resolves a device path to a stable identifier
// Accepts:
//   - /dev/serial/by-id/* symlinks (returns as-is)
//   - /dev/serial/by-path/* symlinks (converts to by-id)
//   - /dev/ttyUSB*, /dev/ttyACM* (resolves to by-id)
func (r *DeviceResolver) ResolvePath(path string) (string, error) {
	// Already a stable by-id path
	if strings.HasPrefix(path, "/dev/serial/by-id/") {
		return path, nil
	}

	// Try to find matching by-id symlink
	if strings.HasPrefix(path, "/dev/") {
		byID, err := r.findByIDPath(path)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", path, err)
		}
		if byID != "" {
			return byID, nil
		}
		// Fallback: return original path if no by-id found
		log.Warn().Str("path", path).Msg("No by-id symlink found, using unstable path")
		return path, nil
	}

	return "", fmt.Errorf("invalid device path: %s", path)
}

// GetDeviceInfo extracts device information for a path
func (r *DeviceResolver) GetDeviceInfo(path string) (*DeviceInfo, error) {
	r.mu.RLock()
	if info, exists := r.cache[path]; exists {
		r.mu.RUnlock()
		return info, nil
	}
	r.mu.RUnlock()

	// Resolve to actual device path (resolve symlinks)
	realPath := path
	if strings.HasPrefix(path, "/dev/serial/by-id/") {
		// Resolve the symlink to get the real path
		target, err := os.Readlink(path)
		if err == nil {
			realPath = filepath.Join(filepath.Dir(path), target)
		}
	}

	// Try extracting from udev (primary method for Linux)
	info, err := r.extractFromUdev(realPath)
	if err != nil {
		log.Debug().Str("path", realPath).Err(err).Msg("Failed to extract from udev")
		// Fallback: return basic info
		info = &DeviceInfo{Path: path}
	}

	// Use the original by-id path if that's what was requested
	if strings.HasPrefix(path, "/dev/serial/by-id/") {
		info.Path = path
	}

	r.mu.Lock()
	r.cache[path] = info
	r.mu.Unlock()

	return info, nil
}

// findByIDPath finds the by-id symlink for a given device path
func (r *DeviceResolver) findByIDPath(devicePath string) (string, error) {
	byIDDir := "/dev/serial/by-id"
	entries, err := os.ReadDir(byIDDir)
	if err != nil {
		return "", fmt.Errorf("read by-id directory: %w", err)
	}

	for _, entry := range entries {
		symlink := filepath.Join(byIDDir, entry.Name())
		target, err := os.Readlink(symlink)
		if err != nil {
			continue
		}

		// Resolve relative symlink target
		resolved := filepath.Join(filepath.Dir(symlink), target)
		if resolved == devicePath || filepath.Base(resolved) == filepath.Base(devicePath) {
			return symlink, nil
		}
	}

	return "", fmt.Errorf("no by-id symlink found for %s", devicePath)
}

// extractFromSymlink extracts device info from by-id symlink name
// Format: usb-<vendor>_<model>_<serial>-if<interface>
func (r *DeviceResolver) extractFromSymlink(byIDPath string) *DeviceInfo {
	filename := filepath.Base(byIDPath)

	// Parse usb-VENDOR_MODEL_SERIAL-ifINTERFACE format
	parts := strings.Split(filename, "-")
	if len(parts) < 3 || parts[0] != "usb" {
		return nil
	}

	info := &DeviceInfo{
		Path: byIDPath,
	}

	// Extract serial (last part before -if)
	for i := len(parts) - 1; i >= 2; i-- {
		if strings.HasPrefix(parts[i], "if") {
			// Found interface marker, serial is before this
			if i > 2 {
				info.SerialNumber = strings.ReplaceAll(parts[i-1], "_", ":")
			}
			break
		}
	}

	// Extract vendor and model (parts[1] and parts[2])
	// Model might contain underscores, need to find where model ends and serial begins
	// This is complex due to variable format, skip for now
	// Will rely on udev extraction instead

	return info
}

// extractFromUdev extracts device info using udevadm
func (r *DeviceResolver) extractFromUdev(path string) (*DeviceInfo, error) {
	info := &DeviceInfo{Path: path}

	// Try to get udev properties
	props, err := getUdevProperties(path)
	if err != nil {
		return nil, err
	}

	// Extract properties
	if serial, ok := props["ID_SERIAL_SHORT"]; ok {
		info.SerialNumber = serial
	}
	if vendor, ok := props["ID_VENDOR"]; ok {
		info.Manufacturer = vendor
	}
	if model, ok := props["ID_MODEL"]; ok {
		info.Product = model
	}
	if vidStr, ok := props["ID_VENDOR_ID"]; ok {
		if vid, err := strconv.ParseInt(vidStr, 16, 16); err == nil {
			info.VID = uint16(vid)
		}
	}
	if pidStr, ok := props["ID_MODEL_ID"]; ok {
		if pid, err := strconv.ParseInt(pidStr, 16, 16); err == nil {
			info.PID = uint16(pid)
		}
	}
	if pathTag, ok := props["ID_PATH_TAG"]; ok {
		info.Location = pathTag
	}

	return info, nil
}

// ScanStable scans all devices by-id and returns device info
func (r *DeviceResolver) ScanStable() ([]DeviceInfo, error) {
	byIDDir := "/dev/serial/by-id"
	entries, err := os.ReadDir(byIDDir)
	if err != nil {
		return nil, fmt.Errorf("read by-id directory: %w", err)
	}

	result := make([]DeviceInfo, 0, len(entries))
	for _, entry := range entries {
		symlink := filepath.Join(byIDDir, entry.Name())
		target, err := os.Readlink(symlink)
		if err != nil {
			log.Debug().Str("symlink", symlink).Err(err).Msg("Failed to read symlink")
			continue
		}

		// Resolve to actual device
		resolved := filepath.Join(filepath.Dir(symlink), target)

		// Extract device info
		info, err := r.GetDeviceInfo(resolved)
		if err != nil {
			log.Debug().Str("device", resolved).Err(err).Msg("Failed to get device info")
			continue
		}

		// Use by-id path as primary
		info.Path = symlink
		result = append(result, *info)
	}

	return result, nil
}

// InvalidateCache clears the device info cache
func (r *DeviceResolver) InvalidateCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]*DeviceInfo)
}

// getUdevProperties retrieves udev properties for a device
func getUdevProperties(path string) (map[string]string, error) {
	props := make(map[string]string)

	// Try reading from sysfs for USB devices
	if strings.HasPrefix(path, "/dev/ttyUSB") || strings.HasPrefix(path, "/dev/ttyACM") {
		baseName := filepath.Base(path)
		deviceLink := fmt.Sprintf("/sys/class/tty/%s/device", baseName)

		// Resolve the symlink to get the interface path
		interfacePath, err := filepath.EvalSymlinks(deviceLink)
		if err != nil {
			return props, nil
		}

		// Strip interface suffix (e.g., "1-5.4.3:1.0" -> "1-5.4.3")
		devicePath := interfacePath
		if idx := strings.LastIndex(interfacePath, ":"); idx != -1 {
			devicePath = interfacePath[:idx]
		}

		// Walk up from device to find the actual USB device (skipping hubs)
		usbPath, err := findUSBDevicePath(devicePath)
		if err == nil {
			// Read device attributes
			if vid, err := os.ReadFile(filepath.Join(usbPath, "idVendor")); err == nil {
				props["ID_VENDOR_ID"] = strings.TrimSpace(string(vid))
			}
			if pid, err := os.ReadFile(filepath.Join(usbPath, "idProduct")); err == nil {
				props["ID_MODEL_ID"] = strings.TrimSpace(string(pid))
			}
			if serial, err := os.ReadFile(filepath.Join(usbPath, "serial")); err == nil {
				props["ID_SERIAL_SHORT"] = strings.TrimSpace(string(serial))
			}
			if manufacturer, err := os.ReadFile(filepath.Join(usbPath, "manufacturer")); err == nil {
				props["ID_VENDOR"] = strings.TrimSpace(string(manufacturer))
			}
			if product, err := os.ReadFile(filepath.Join(usbPath, "product")); err == nil {
				props["ID_MODEL"] = strings.TrimSpace(string(product))
			}
		}
	}

	return props, nil
}

// findUSBDevicePath walks up from device directory to find the actual USB device
// Skips USB hubs to find the target device
func findUSBDevicePath(startPath string) (string, error) {
	current := startPath

	for i := 0; i < 15; i++ { // Limit depth
		// Check if this is a USB device (has idVendor file)
		vidPath := filepath.Join(current, "idVendor")
		if vidData, err := os.ReadFile(vidPath); err == nil {
			vid := strings.TrimSpace(string(vidData))
			// Skip USB hubs (common hub VIDs)
			if !isHubVID(vid) {
				return current, nil
			}
		}

		// Move to parent
		parent := filepath.Dir(current)
		if parent == current || parent == "/sys" || parent == "/sys/devices" {
			break
		}
		current = parent
	}

	return "", fmt.Errorf("USB device not found")
}

// isHubVID checks if the VID is a known USB hub vendor
func isHubVID(vid string) bool {
	hubVIDs := []string{
		"1d6b", // Linux Foundation
		"0bda", // Realtek (common hubs)
		"0424", // Standard Microsystems Corp (hubs)
		"05e3", // Genesys Logic (hubs)
		"1a40", // Terminus Technology (hubs)
	}
	for _, hubVID := range hubVIDs {
		if vid == hubVID {
			return true
		}
	}
	return false
}
