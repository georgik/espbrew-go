//go:build linux
// +build linux

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/vladimirvivien/go4vl/device"
	v4l2 "github.com/vladimirvivien/go4vl/v4l2"
)

func main() {
	devicePath := "/dev/video0"
	if len(os.Args) > 1 {
		devicePath = os.Args[1]
	}

	fmt.Printf("=== Camera Control Test ===\n")
	fmt.Printf("Device: %s\n\n", devicePath)

	// Open device
	dev, err := device.Open(devicePath)
	if err != nil {
		log.Fatalf("Failed to open device: %v", err)
	}
	defer dev.Close()

	fmt.Println("✓ Device opened successfully")

	// Query and display current control values
	fmt.Println("\n=== Current Camera Settings ===")
	displayCurrentSettings(dev)

	// Test setting controls for display photography
	fmt.Println("\n=== Applying Display-Optimized Settings ===")
	applyDisplaySettings(dev)

	// Verify new settings
	fmt.Println("\n=== Settings After Adjustment ===")
	displayCurrentSettings(dev)

	// Test capture with new settings
	fmt.Println("\n=== Testing Capture ===")
	testCapture(dev)
}

func displayCurrentSettings(dev *device.Device) {
	// User controls - use high-level convenience methods
	if brightness, err := dev.GetBrightness(); err == nil {
		fmt.Printf("Brightness: %d\n", brightness)
	}
	if contrast, err := dev.GetContrast(); err == nil {
		fmt.Printf("Contrast: %d\n", contrast)
	}
	if hue, err := dev.GetHue(); err == nil {
		fmt.Printf("Hue: %d\n", hue)
	}

	// Saturation via control query
	if saturation, err := dev.GetControl(v4l2.CtrlSaturation); err == nil {
		fmt.Printf("Saturation: %d\n", saturation.Value)
	}

	// Sharpness via control query
	if sharpness, err := dev.GetControl(v4l2.CtrlSharpness); err == nil {
		fmt.Printf("Sharpness: %d\n", sharpness.Value)
	}

	// Gain via control query
	if gain, err := dev.GetControl(v4l2.CtrlGain); err == nil {
		fmt.Printf("Gain: %d\n", gain.Value)
	}

	// Camera controls via extended controls
	displayCameraControls(dev)
}

func displayCameraControls(dev *device.Device) {
	// Get current camera controls
	ctrls := v4l2.NewExtControls()
	ctrls.Add(v4l2.NewExtControl(v4l2.CtrlCameraFocusAbsolute))
	ctrls.Add(v4l2.NewExtControl(v4l2.CtrlCameraExposureAbsolute))
	ctrls.Add(v4l2.NewExtControl(v4l2.CtrlCameraExposureAuto))
	ctrls.Add(v4l2.NewExtControl(v4l2.CtrlCameraFocusAuto))

	err := dev.GetExtControls(ctrls)
	if err != nil {
		fmt.Printf("  (Extended camera controls: %v)\n", err)
		return
	}

	for _, ctrl := range ctrls.GetControls() {
		switch ctrl.ID {
		case v4l2.CtrlCameraFocusAbsolute:
			fmt.Printf("Focus (absolute): %d\n", ctrl.Value)
		case v4l2.CtrlCameraExposureAbsolute:
			fmt.Printf("Exposure (absolute): %d\n", ctrl.Value)
		case v4l2.CtrlCameraExposureAuto:
			fmt.Printf("Auto Exposure: %d\n", ctrl.Value)
		case v4l2.CtrlCameraFocusAuto:
			fmt.Printf("Auto Focus: %d\n", ctrl.Value)
		}
	}
}

func applyDisplaySettings(dev *device.Device) {
	// Settings optimized for capturing glowing displays:
	// - Lower brightness (avoid overexposure)
	// - Higher contrast (improve text readability)
	// - Moderate sharpness (clear text without artifacts)
	// - Manual exposure (consistent lighting)

	fmt.Println("Applying:")

	// Use high-level convenience methods where available
	if err := dev.SetBrightness(80); err != nil {
		fmt.Printf("  Brightness: 80 (error: %v)\n", err)
	} else {
		fmt.Println("  ✓ Brightness: 80")
	}

	if err := dev.SetContrast(140); err != nil {
		fmt.Printf("  Contrast: 140 (error: %v)\n", err)
	} else {
		fmt.Println("  ✓ Contrast: 140")
	}

	// Saturation via control value
	if err := dev.SetControlSaturation(90); err != nil {
		fmt.Printf("  Saturation: 90 (error: %v)\n", err)
	} else {
		fmt.Println("  ✓ Saturation: 90")
	}

	// Sharpness via control value (need to convert to CtrlValue)
	if err := dev.SetControlValue(v4l2.CtrlSharpness, 150); err != nil {
		fmt.Printf("  Sharpness: 150 (error: %v)\n", err)
	} else {
		fmt.Println("  ✓ Sharpness: 150")
	}

	// Camera controls for focus and exposure
	applyCameraControls(dev)
}

func applyCameraControls(dev *device.Device) {
	// Set manual focus to a good distance for displays
	// Set manual exposure for consistent results

	ctrls := v4l2.NewExtControls()

	// Disable auto focus, set fixed focus for typical display distance (~0.5-2m)
	ctrls.AddValue(v4l2.CtrlCameraFocusAuto, 0)      // Disable auto focus
	ctrls.AddValue(v4l2.CtrlCameraFocusAbsolute, 85) // Fixed focus position

	// Set manual exposure mode
	ctrls.AddValue(v4l2.CtrlCameraExposureAuto, 1)       // Manual exposure mode
	ctrls.AddValue(v4l2.CtrlCameraExposureAbsolute, 300) // Moderate exposure

	err := dev.SetExtControls(ctrls)
	if err != nil {
		fmt.Printf("  Camera controls: %v\n", err)
	} else {
		fmt.Println("  ✓ Manual focus and exposure set")
	}
}

func testCapture(dev *device.Device) {
	// Get current format
	format, err := dev.GetPixFormat()
	if err != nil {
		log.Printf("Failed to get format: %v", err)
		return
	}

	fmt.Printf("Format: 0x%08x (%dx%d)\n", uint32(format.PixelFormat), format.Width, format.Height)

	// Try to capture a single frame
	ctx := context.Background()
	if err := dev.Start(ctx); err != nil {
		log.Printf("Failed to start capture: %v", err)
		return
	}
	defer dev.Stop()

	fmt.Println("✓ Capture started, waiting for frame...")

	// Wait for first frame (with timeout)
	select {
	case frame := <-dev.GetOutput():
		fmt.Printf("✓ Captured frame: %d bytes\n", len(frame))
	case <-ctx.Done():
		fmt.Println("✗ Capture timeout")
	}
}
