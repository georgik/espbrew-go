//go:build js
// +build js

package api

import (
	"syscall/js"
)

// GetDevices retrieves the list of devices
func GetDevices(callback func([]Device, error)) {
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

	return Device{
		DeviceID:   ValueToString(v.Get("device_id")),
		Path:       ValueToString(v.Get("path")),
		ChipType:   ValueToString(v.Get("chip_type")),
		Status:     ValueToString(v.Get("status")),
		Aliases:    aliases,
		MACAddress: ValueToString(v.Get("mac_address")),
		NodeID:     ValueToString(v.Get("node_id")),
		Protected:  ValueToBool(v.Get("protected")),
	}
}

// GetDevice retrieves a single device by ID
func GetDevice(deviceID string, callback func(*Device, error)) {
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
	DefaultAsyncClient.Post("/devices/"+deviceID+"/protect", nil, func(result js.Value, err error) {
		callback(err)
	})
}

// UnprotectDevice unprotects a device
func UnprotectDevice(deviceID string, callback func(error)) {
	DefaultAsyncClient.Post("/devices/"+deviceID+"/unprotect", nil, func(result js.Value, err error) {
		callback(err)
	})
}

// DisableDevice disables a device
func DisableDevice(deviceID string, callback func(error)) {
	DefaultAsyncClient.Post("/devices/"+deviceID+"/disable", nil, func(result js.Value, err error) {
		callback(err)
	})
}

// EnableDevice enables a device
func EnableDevice(deviceID string, callback func(error)) {
	DefaultAsyncClient.Post("/devices/"+deviceID+"/enable", nil, func(result js.Value, err error) {
		callback(err)
	})
}

// UpdateDevice updates device attributes
func UpdateDevice(deviceID string, attrs map[string]interface{}, callback func(bool, error)) {
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
