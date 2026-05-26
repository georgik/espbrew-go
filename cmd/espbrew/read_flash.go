package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/flash"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"go.bug.st/serial"
)

var readFlashCmd struct {
	clusterURL string
	device     string
	address    uint32
	size       uint32
	chip       string
}

func init() {
	readCmd := &cobra.Command{
		Use:   "read-flash [output-file]",
		Short: "Read data from ESP flash memory",
		Long: `Read data from ESP device flash memory and save to a file.

The read operation can be performed locally or via a cluster server.
Local mode connects directly to the device.
Cluster mode sends the read request to a cluster server.

Examples:
  espbrew read-flash --address 0x10000 --size 0x100000 app.bin
  espbrew read-flash --cluster http://localhost:8080 --device /dev/ttyUSB0 --address 0x10000 --size 0x100000 app.bin`,
		RunE: runReadFlash,
	}

	rootCmd.AddCommand(readCmd)

	readCmd.Flags().StringVar(&readFlashCmd.clusterURL, "cluster", "", "Cluster URL for remote reading")
	readCmd.Flags().StringVar(&readFlashCmd.device, "device", "", "Device path (required with cluster)")
	readCmd.Flags().Uint32Var(&readFlashCmd.address, "address", 0x10000, "Flash address to read from")
	readCmd.Flags().Uint32Var(&readFlashCmd.size, "size", 0x100000, "Number of bytes to read")
	readCmd.Flags().StringVar(&readFlashCmd.chip, "chip", "auto", "Chip type (esp32, esp32s3, esp32c3, etc.)")
}

func runReadFlash(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("output file path required")
	}

	outputFile := args[0]

	if readFlashCmd.size == 0 {
		return fmt.Errorf("size must be greater than 0")
	}

	const maxSize = 16 * 1024 * 1024
	if readFlashCmd.size > maxSize {
		return fmt.Errorf("size exceeds maximum of %d bytes", maxSize)
	}

	log.Info().Str("output", outputFile).Uint32("address", readFlashCmd.address).Uint32("size", readFlashCmd.size).Msg("Read flash parameters")

	if readFlashCmd.clusterURL != "" {
		return runReadFlashCluster(outputFile)
	}

	return runReadFlashLocal(outputFile)
}

func runReadFlashLocal(outputFile string) error {
	port, err := findDevicePort()
	if err != nil {
		return fmt.Errorf("find device: %w", err)
	}

	log.Info().Str("port", port).Msg("Using local device")

	flasher := flash.NewFlasher(nil)

	req := &flash.ReadFlashRequest{
		Port:    port,
		Address: readFlashCmd.address,
		Size:    readFlashCmd.size,
	}

	ctx := context.Background()
	result := flasher.ReadFlash(ctx, req)

	if !result.Success {
		return fmt.Errorf("read flash failed: %w", result.Error)
	}

	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := os.WriteFile(outputFile, result.Data, 0644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	log.Info().Str("file", outputFile).Int("bytes", len(result.Data)).Msg("Flash data saved successfully")
	printHexDump(result.Data, 64)

	return nil
}

func findDevicePort() (string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return "", fmt.Errorf("list ports: %w", err)
	}

	if len(ports) == 0 {
		return "", fmt.Errorf("no serial ports found")
	}

	// Prefer USB-serial ports over Bluetooth
	for _, port := range ports {
		lower := strings.ToLower(port)
		if strings.Contains(lower, "usb") || strings.Contains(lower, "acm") {
			return port, nil
		}
	}

	// Fall back to cu.* or tty.* but skip known Bluetooth ports
	for _, port := range ports {
		lower := strings.ToLower(port)
		if (strings.Contains(lower, "cu.") || strings.Contains(lower, "tty.")) &&
			!strings.Contains(lower, "bluetooth") {
			return port, nil
		}
	}

	log.Debug().Msg("No ESP-specific port found, using first available port")
	return ports[0], nil
}

func runReadFlashCluster(outputFile string) error {
	if readFlashCmd.device == "" {
		return fmt.Errorf("device path required when using cluster mode")
	}

	client := cluster.NewClient(readFlashCmd.clusterURL)

	log.Info().Str("cluster", readFlashCmd.clusterURL).Str("device", readFlashCmd.device).Msg("Submitting read job to cluster")

	req := cluster.ReadFlashRequest{
		DevicePath: readFlashCmd.device,
		Address:    readFlashCmd.address,
		Size:       readFlashCmd.size,
	}

	resp, err := client.ReadFlash(req)
	if err != nil {
		return fmt.Errorf("submit read job: %w", err)
	}

	log.Info().Str("job_id", resp.JobID).Msg("Read job submitted")

	status := resp
	for status.Status == "pending" || status.Status == "running" {
		status, err = client.GetReadFlashStatus(resp.JobID)
		if err != nil {
			return fmt.Errorf("get job status: %w", err)
		}
		log.Debug().Str("status", status.Status).Msg("Job status")
	}

	if status.Status != "completed" {
		return fmt.Errorf("read job failed: %s", status.Error)
	}

	log.Info().Str("job_id", resp.JobID).Int64("bytes", status.Size).Msg("Read job completed")

	data, err := client.DownloadReadFlash(resp.JobID)
	if err != nil {
		return fmt.Errorf("download read data: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	log.Info().Str("file", outputFile).Int("bytes", len(data)).Msg("Flash data saved successfully")
	printHexDump(data, 64)

	return nil
}

func printHexDump(data []byte, lines int) {
	const bytesPerLine = 16

	maxLines := len(data) / bytesPerLine
	if maxLines > lines {
		maxLines = lines
	}

	log.Info().Msg("Hex dump (first bytes):")
	for i := 0; i < maxLines; i++ {
		offset := i * bytesPerLine
		end := offset + bytesPerLine
		if end > len(data) {
			end = len(data)
		}

		hexStr := ""
		asciiStr := ""
		for j := offset; j < end; j++ {
			hexStr += fmt.Sprintf("%02x ", data[j])
			if data[j] >= 32 && data[j] <= 126 {
				asciiStr += string(data[j])
			} else {
				asciiStr += "."
			}
		}

		log.Info().Msgf("  0x%04x: %-48s %s", offset, hexStr, asciiStr)
	}
}
