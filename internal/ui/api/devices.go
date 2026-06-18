//go:build js
// +build js

package api

import (
	"syscall/js"
)

// GetDevices retrieves the list of devices
func GetDevices(callback func([]Device, error)) {
	if DemoModeEnabled() {
		callback(mockDevices(), nil)
		return
	}

	DefaultAsyncClient.Get("/devices", func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		// Result is the array directly, not wrapped in a "devices" object
		if result.IsUndefined() || result.IsNull() {
			callback([]Device{}, nil)
			return
		}

		devices := parseDevicesArray(result)
		callback(devices, nil)
	})
}

// parseDevicesArray parses a js.Value array into Device slice
func parseDevicesArray(arr js.Value) []Device {
	length := arr.Get("length").Int()
	devices := make([]Device, length)

	for i := 0; i < length; i++ {
		devices[i] = parseDevice(arr.Index(i))
	}

	return devices
}

// parseDevice parses a js.Value into Device struct
func parseDevice(v js.Value) Device {
	// Parse aliases array
	aliasesArray := v.Get("aliases")
	var aliases []string
	if !aliasesArray.IsUndefined() && !aliasesArray.IsNull() {
		aliasesLength := aliasesArray.Get("length").Int()
		aliases = make([]string, aliasesLength)
		for i := 0; i < aliasesLength; i++ {
			aliases[i] = ValueToString(aliasesArray.Index(i))
		}
	}

	// Parse backend config
	var backendConfig map[string]interface{}
	if backendConfigVal := v.Get("backend_config"); !backendConfigVal.IsUndefined() && !backendConfigVal.IsNull() {
		backendConfig = make(map[string]interface{})
		// Use ParseJSONValue to parse the nested object
		if err := ParseJSONValue(backendConfigVal, &backendConfig); err != nil {
			// If parsing fails, leave backendConfig empty
		}
	}

	return Device{
		DeviceID:      ValueToString(v.Get("device_id")),
		Path:          ValueToString(v.Get("path")),
		ChipType:      ValueToString(v.Get("chip_type")),
		Status:        ValueToString(v.Get("status")),
		Aliases:       aliases,
		MACAddress:    ValueToString(v.Get("mac_address")),
		NodeID:        ValueToString(v.Get("node_id")),
		Protected:     ValueToBool(v.Get("protected")),
		Backend:       ValueToString(v.Get("backend")),
		BackendConfig: backendConfig,
	}
}

// GetDevice retrieves a single device by ID
func GetDevice(deviceID string, callback func(*Device, error)) {
	if DemoModeEnabled() {
		callback(mockDevice(deviceID), nil)
		return
	}

	DefaultAsyncClient.Get("/devices/"+deviceID, func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		device := &Device{}
		if err := ParseJSONValue(result, device); err != nil {
			callback(nil, err)
			return
		}

		callback(device, nil)
	})
}

// ProtectDevice protects a device from flashing
func ProtectDevice(deviceID string, callback func(error)) {
	if DemoModeEnabled() {
		callback(nil)
		return
	}

	DefaultAsyncClient.Post("/devices/"+deviceID+"/protect", nil, func(result js.Value, err error) {
		callback(err)
	})
}

// UnprotectDevice unprotects a device
func UnprotectDevice(deviceID string, callback func(error)) {
	if DemoModeEnabled() {
		callback(nil)
		return
	}

	DefaultAsyncClient.Post("/devices/"+deviceID+"/unprotect", nil, func(result js.Value, err error) {
		callback(err)
	})
}

// DeleteDevice deletes a device
func DeleteDevice(deviceID string, callback func(bool, error)) {
	if DemoModeEnabled() {
		callback(true, nil)
		return
	}

	DefaultAsyncClient.Delete("/devices/"+deviceID, func(result js.Value, err error) {
		if err != nil {
			callback(false, err)
			return
		}
		// Delete returns 204 No Content on success
		callback(true, nil)
	})
}

// DisableDevice disables a device
func DisableDevice(deviceID string, callback func(error)) {
	if DemoModeEnabled() {
		callback(nil)
		return
	}

	DefaultAsyncClient.Post("/devices/"+deviceID+"/disable", nil, func(result js.Value, err error) {
		callback(err)
	})
}

// EnableDevice enables a device
func EnableDevice(deviceID string, callback func(error)) {
	if DemoModeEnabled() {
		callback(nil)
		return
	}

	DefaultAsyncClient.Post("/devices/"+deviceID+"/enable", nil, func(result js.Value, err error) {
		callback(err)
	})
}

// UpdateDevice updates device attributes
func UpdateDevice(deviceID string, attrs map[string]interface{}, callback func(bool, error)) {
	if DemoModeEnabled() {
		callback(true, nil)
		return
	}

	DefaultAsyncClient.Patch("/devices/"+deviceID, attrs, func(result js.Value, err error) {
		if err != nil {
			callback(false, err)
			return
		}

		// Check for success status
		if !result.IsUndefined() && !result.IsNull() {
			status := ValueToString(result.Get("status"))
			success := (status == "ok" || status == "updated")
			callback(success, nil)
			return
		}

		callback(false, nil)
	})
}

// SetBackendConfig updates device backend configuration
func SetBackendConfig(deviceID string, backend string, backendConfig map[string]interface{}, callback func(bool, error)) {
	if DemoModeEnabled() {
		callback(true, nil)
		return
	}

	req := map[string]interface{}{
		"backend":        backend,
		"backend_config": backendConfig,
	}
	DefaultAsyncClient.Put("/devices/"+deviceID+"/backend", req, func(result js.Value, err error) {
		if err != nil {
			callback(false, err)
			return
		}

		// Check for success status
		if !result.IsUndefined() && !result.IsNull() {
			deviceIDResult := ValueToString(result.Get("device_id"))
			success := (deviceIDResult != "")
			callback(success, nil)
			return
		}

		callback(false, nil)
	})
}

// ProbeDevice probes a device by path to identify it
func ProbeDevice(path string, callback func(bool, string, string, error)) {
	if DemoModeEnabled() {
		// Mock probe response for common paths
		var success bool
		var deviceID, chipType string
		switch path {
		case "/dev/ttyUSB0":
			success, deviceID, chipType = true, "esp32-devkit-a", "ESP32"
		case "/dev/ttyUSB1":
			success, deviceID, chipType = true, "esp32-cam-001", "ESP32"
		case "/dev/ttyUSB2":
			success, deviceID, chipType = true, "esp8266-generic", "ESP8266"
		default:
			success, deviceID, chipType = false, "", ""
		}
		callback(success, deviceID, chipType, nil)
		return
	}

	req := map[string]interface{}{
		"path": path,
	}
	DefaultAsyncClient.Post("/devices/probe", req, func(result js.Value, err error) {
		if err != nil {
			callback(false, "", "", err)
			return
		}

		// Parse response
		status := ValueToString(result.Get("status"))
		deviceID := ValueToString(result.Get("device_id"))
		chipType := ValueToString(result.Get("chip_type"))

		success := (status == "probed" && deviceID != "")
		callback(success, deviceID, chipType, nil)
	})
}

// ForgetDevice removes an unidentified device from cluster state by path
func ForgetDevice(path string, callback func(bool, error)) {
	if DemoModeEnabled() {
		callback(true, nil)
		return
	}

	DefaultAsyncClient.Delete("/devices/forgot/"+path, func(result js.Value, err error) {
		if err != nil {
			callback(false, err)
			return
		}

		// Check for success status
		if !result.IsUndefined() && !result.IsNull() {
			status := ValueToString(result.Get("status"))
			success := (status == "forgotten")
			callback(success, nil)
			return
		}

		callback(false, nil)
	})
}
