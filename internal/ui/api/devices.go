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

	// Parse tags array
	tagsArray := v.Get("tags")
	var tags []string
	if !tagsArray.IsUndefined() && !tagsArray.IsNull() {
		tagsLength := tagsArray.Get("length").Int()
		tags = make([]string, tagsLength)
		for i := 0; i < tagsLength; i++ {
			tags[i] = ValueToString(tagsArray.Index(i))
		}
	}

	return Device{
		DeviceID:      ValueToString(v.Get("device_id")),
		Path:          ValueToString(v.Get("path")),
		RealPath:      ValueToString(v.Get("real_path")),
		ChipType:      ValueToString(v.Get("chip_type")),
		ChipRev:       ValueToString(v.Get("chip_rev")),
		FlashSize:     uint32(ValueToInt(v.Get("flash_size"))),
		PSRAMSize:     uint32(ValueToInt(v.Get("psram_size"))),
		PSRAMType:     ValueToString(v.Get("psram_type")),
		BoardModel:    ValueToString(v.Get("board_model")),
		Description:   ValueToString(v.Get("description")),
		Status:        ValueToString(v.Get("status")),
		Aliases:       aliases,
		Tags:          tags,
		MACAddress:    ValueToString(v.Get("mac_address")),
		NodeID:        ValueToString(v.Get("node_id")),
		Protected:     ValueToBool(v.Get("protected")),
		Disabled:      ValueToBool(v.Get("disabled")),
		AccessError:   ValueToString(v.Get("access_error")),
		Backend:       ValueToString(v.Get("backend")),
		BackendConfig: backendConfig,
		VID:           ValueToString(v.Get("vid")),
		PID:           ValueToString(v.Get("pid")),
		SerialNumber:  ValueToString(v.Get("serial")),
		Manufacturer:  ValueToString(v.Get("manufacturer")),
		Product:       ValueToString(v.Get("product")),
		Connected:     ValueToBool(v.Get("connected")),
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

// ResetDevice triggers a hardware reset on the specified device
func ResetDevice(path string, callback func(bool, error)) {
	if DemoModeEnabled() {
		// In demo mode, simulate reset
		js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			callback(true, nil)
			return nil
		}), 500)
		return
	}

	DefaultAsyncClient.Post("/devices/reset", map[string]interface{}{
		"path": path,
	}, func(result js.Value, err error) {
		if err != nil {
			callback(false, err)
			return
		}

		// Check for success status
		if !result.IsUndefined() && !result.IsNull() {
			status := ValueToString(result.Get("status"))
			success := (status == "reset" || status == "ok")
			callback(success, nil)
			return
		}

		callback(false, nil)
	})
}

// CreateDevice creates a new device record (for manually adding unprobed devices)
func CreateDevice(data map[string]interface{}, callback func(bool, string, error)) {
	if DemoModeEnabled() {
		// In demo mode, generate mock device ID
		callback(true, "demo-device-"+ValueToString(js.Global().Get("Date").Call("now")), nil)
		return
	}

	DefaultAsyncClient.Post("/devices", data, func(result js.Value, err error) {
		if err != nil {
			callback(false, "", err)
			return
		}

		// Parse response for device_id
		deviceID := ValueToString(result.Get("device_id"))
		if deviceID == "" {
			deviceID = ValueToString(result.Get("id"))
		}
		success := (deviceID != "")
		callback(success, deviceID, nil)
	})
}
