// Package snap provides device snapshot functionality for ESP32 firmware development.
//
// A snapshot captures the complete state of a target device, including:
//   - Serial monitor logs during/after flashing
//   - Camera images of physical device displays
//   - Flash memory state before/after operations
//   - Device metadata (chip type, port, timestamps)
//
// Basic Usage:
//
//	executor := snap.NewExecutor("/dev/ttyUSB0", "firmware.bin", 10*time.Second)
//	executor.BaudRate = 460800
//	executor.CameraID = "/dev/video0"
//
//	result, err := executor.Run(context.Background())
//	if err != nil {
//	    log.Error().Err(err).Msg("Snapshot failed")
//	}
//
//	// Access results
//	logs := result.Logs
//	imageData := result.ImageData
//	metadata := result.Metadata
//
// Flags for controlling behavior:
//   - SkipFlash: Skip firmware flashing entirely
//   - ForceFlash: Flash even if firmware hash matches existing flash
//   - NoCapture: Disable camera capture
//   - NoMonitor: Disable serial monitoring
//
// The executor coordinates multiple concurrent operations:
//  1. Firmware flashing with hash verification
//  2. Device reset and boot
//  3. Serial log capture with level parsing
//  4. Camera image capture
//
// All operations respect context cancellation for graceful shutdown.
package snap
