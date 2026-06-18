//go:build js
// +build js

package api

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// GetMode retrieves the current operational mode
func GetMode(callback func(mode OperationMode, err error)) {
	if DemoModeEnabled() {
		callback(ModeOperational, nil)
		return
	}

	DefaultAsyncClient.Get("/mode", func(result js.Value, err error) {
		if err != nil {
			callback("", err)
			return
		}

		modeStr := ValueToString(result.Get("mode"))
		callback(OperationMode(modeStr), nil)
	})
}

// SetMode sets the operational mode
func SetMode(mode OperationMode, callback func(success bool, err error)) {
	if DemoModeEnabled() {
		callback(true, nil)
		return
	}

	req := ModeRequest{Mode: mode}
	DefaultAsyncClient.Put("/mode", req, func(result js.Value, err error) {
		if err != nil {
			callback(false, err)
			return
		}
		callback(true, nil)
	})
}

// RefreshDiscovery forces a device re-scan in discovery mode
func RefreshDiscovery(callback func(success bool, err error)) {
	if DemoModeEnabled() {
		callback(true, nil)
		return
	}

	DefaultAsyncClient.Post("/discovery/refresh", nil, func(result js.Value, err error) {
		if err != nil {
			callback(false, err)
			return
		}
		callback(true, nil)
	})
}

// GetStatus retrieves cluster status including mode
func GetStatus(callback func(status *StatusResponse, err error)) {
	if DemoModeEnabled() {
		callback(mockStatus(), nil)
		return
	}

	DefaultAsyncClient.Get("/status", func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		status := &StatusResponse{}
		if parseErr := ParseJSONValue(result, status); parseErr != nil {
			callback(nil, parseErr)
			return
		}

		callback(status, nil)
	})
}

// GetCurrentMode is a helper that returns the current mode from status
func GetCurrentMode(callback func(mode OperationMode, err error)) {
	GetStatus(func(status *StatusResponse, err error) {
		if err != nil {
			callback("", err)
			return
		}
		callback(status.Mode, nil)
	})
}

// ModeDisplay returns human-readable mode name
func ModeDisplay(mode OperationMode) string {
	switch mode {
	case ModeDiscovery:
		return "Discovery"
	case ModeOperational:
		return "Operational"
	default:
		return "Unknown"
	}
}

// ModeDescription returns mode description
func ModeDescription(mode OperationMode) string {
	switch mode {
	case ModeDiscovery:
		return "Device detection active"
	case ModeOperational:
		return "Flashing enabled"
	default:
		return ""
	}
}

// ModeColor returns CSS color for mode badge
func ModeColor(mode OperationMode) string {
	switch mode {
	case ModeDiscovery:
		return "#ffa502"
	case ModeOperational:
		return "#4cd137"
	default:
		return "#aaa"
	}
}

// ModeBadge creates a mode badge element
func ModeBadge(mode OperationMode) *dom.Element {
	doc := dom.GlobalDocument()
	badge := doc.CreateElement("span")
	badge.SetStyle("display", "inline-block")
	badge.SetStyle("padding", "4px 8px")
	badge.SetStyle("border-radius", "4px")
	badge.SetStyle("font-size", "11px")
	badge.SetStyle("font-weight", "500")
	badge.SetStyle("background-color", "rgba("+colorToRGBA(ModeColor(mode))+",0.2)")
	badge.SetStyle("color", ModeColor(mode))
	badge.SetTextContent(ModeDisplay(mode))
	return badge
}

// colorToRGBA converts hex color to rgba format
func colorToRGBA(hex string) string {
	switch hex {
	case "#ffa502":
		return "255, 165, 2"
	case "#4cd137":
		return "76, 209, 55"
	case "#aaa":
		return "170, 170, 170"
	default:
		return "170, 170, 170"
	}
}
