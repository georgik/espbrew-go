package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/device"
	"codeberg.org/georgik/espbrew-go/internal/flash"
	"codeberg.org/georgik/espbrew-go/internal/inventory"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// resolveEraseDevice resolves device identifier to port path using inventory
func resolveEraseDevice() (string, error) {
	if eraseOpts.deviceID == "" {
		return "", fmt.Errorf("no device identifier specified")
	}

	inv, err := inventory.NewInventory()
	if err != nil {
		return "", fmt.Errorf("load inventory: %w", err)
	}

	dev, err := findDevice(inv, eraseOpts.deviceID)
	if err != nil {
		return "", err
	}

	// Get last known path
	if dev.LastPath == "" {
		return "", fmt.Errorf("device %s has no recorded path (probe device first)", dev.DeviceID)
	}

	log.Info().Str("device_id", dev.DeviceID).Str("path", dev.LastPath).Msg("Resolved device from inventory")
	return dev.LastPath, nil
}

var eraseCmd = &cobra.Command{
	Use:   "erase",
	Short: "Erase ESP device flash memory",
	RunE:  runErase,
}

var eraseOpts struct {
	clusterURL string
	deviceID   string
	port       string
	address    string
	size       string
	eraseAll   bool
	chip       string
}

func init() {
	eraseCmd.Flags().StringVar(&eraseOpts.clusterURL, "cluster", os.Getenv("ESPBREW_CLUSTER"), "Cluster URL for remote erase")
	eraseCmd.Flags().StringVar(&eraseOpts.deviceID, "device", "", "Device selection by ID, alias, or MAC (from inventory)")
	eraseCmd.Flags().StringVarP(&eraseOpts.port, "port", "p", "", "Serial port (auto-detect if empty)")
	eraseCmd.Flags().StringVar(&eraseOpts.address, "address", "0", "Start address for region erase (hex format)")
	eraseCmd.Flags().StringVar(&eraseOpts.size, "size", "0", "Size for region erase (hex format)")
	eraseCmd.Flags().BoolVar(&eraseOpts.eraseAll, "all", true, "Erase entire flash")
	eraseCmd.Flags().StringVar(&eraseOpts.chip, "chip", "auto", "Chip type (auto, esp8266, esp32, esp32s2, esp32s3, esp32c3, esp32c6, esp32h2)")

	rootCmd.AddCommand(eraseCmd)
}

func runErase(cmd *cobra.Command, args []string) error {
	if eraseOpts.clusterURL != "" {
		return runEraseRemote()
	}
	return runEraseLocal()
}

func runEraseRemote() error {
	// Resolve device from inventory if --device specified
	if eraseOpts.deviceID != "" {
		port, err := resolveEraseDevice()
		if err != nil {
			return err
		}
		eraseOpts.port = port
	}

	client := cluster.NewClient(eraseOpts.clusterURL)

	// Get available devices if port not specified
	var devicePath string
	if eraseOpts.port == "" {
		devices, err := client.ListDevices()
		if err != nil {
			return fmt.Errorf("list devices: %w", err)
		}

		// Find first available device
		for _, d := range devices {
			if d.State == "available" {
				devicePath = d.Path
				break
			}
		}

		if devicePath == "" {
			return fmt.Errorf("no available devices on cluster")
		}

		log.Info().Str("device", devicePath).Msg("Auto-selected available device")
	} else {
		devicePath = eraseOpts.port
	}

	// Parse address and size
	address, err := parseHex(eraseOpts.address)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	size, err := parseHex(eraseOpts.size)
	if err != nil {
		return fmt.Errorf("invalid size: %w", err)
	}

	// Determine erase mode
	eraseAll := eraseOpts.eraseAll
	if !eraseAll && (address == 0 || size == 0) {
		return fmt.Errorf("for region erase, both --address and --size must be non-zero, or use --all")
	}

	log.Info().Str("cluster", eraseOpts.clusterURL).Str("device", devicePath).
		Bool("erase_all", eraseAll).Uint32("address", address).Uint32("size", size).
		Msg("Submitting erase job to cluster")

	// Submit erase job
	eraseResp, err := client.SubmitErase(cluster.EraseSubmitRequest{
		DevicePath: devicePath,
		Address:    address,
		Size:       size,
		EraseAll:   eraseAll,
		ClientID:   "espbrew-cli",
	})
	if err != nil {
		return fmt.Errorf("submit erase: %w", err)
	}

	log.Info().Str("job_id", eraseResp.JobID).Msg("Erase job submitted, streaming progress...")

	// Connect to progress WebSocket
	progressClient, err := client.ConnectProgress(eraseResp.JobID)
	if err != nil {
		log.Warn().Err(err).Msg("Could not connect to progress WebSocket")
		fmt.Printf("Job ID: %s\n", eraseResp.JobID)
		fmt.Printf("Status: %s\n", eraseResp.Status)
		fmt.Printf("Device: %s\n", eraseResp.DevicePath)
		return nil
	}
	defer progressClient.Close()

	// Stream progress
	lastProgress := -1
	err = progressClient.Stream(func(msg cluster.ProgressMessage) {
		switch msg.Type {
		case "init":
			fmt.Printf("\nJob ID: %s\n", msg.JobID)
			fmt.Printf("Device: %s\n\n", devicePath)
			if eraseAll {
				fmt.Printf("Erasing entire flash...\n")
			} else {
				fmt.Printf("Erasing region: 0x%08x - 0x%08x\n", address, address+size)
			}
		case "progress":
			if msg.Progress > lastProgress {
				displayProgressBar(msg.Progress, msg.Status)
				lastProgress = msg.Progress
			}
		case "complete":
			if msg.Status == "completed" {
				fmt.Printf("\nFlash erased successfully!\n")
			} else {
				fmt.Printf("\nErase failed: %s\n", msg.Error)
			}
		}
	})

	if err != nil {
		return fmt.Errorf("progress stream error: %w", err)
	}

	return nil
}

func runEraseLocal() error {
	// Resolve device from inventory if --device specified
	if eraseOpts.deviceID != "" {
		port, err := resolveEraseDevice()
		if err != nil {
			return err
		}
		eraseOpts.port = port
	}

	if eraseOpts.port == "" {
		scanner := device.NewScanner()
		espPorts, err := scanner.ScanESP()
		if err != nil || len(espPorts) == 0 {
			return fmt.Errorf("--port required or no ESP devices found")
		}
		eraseOpts.port = espPorts[0].Path
		log.Info().Str("auto_port", eraseOpts.port).Msg("Auto-detected ESP device")
	}

	// Parse address and size
	address, err := parseHex(eraseOpts.address)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	size, err := parseHex(eraseOpts.size)
	if err != nil {
		return fmt.Errorf("invalid size: %w", err)
	}

	// Determine erase mode
	eraseAll := eraseOpts.eraseAll
	if !eraseAll && (address == 0 || size == 0) {
		return fmt.Errorf("for region erase, both --address and --size must be non-zero, or use --all")
	}

	flasher := flash.NewFlasher(nil)

	progress := make(chan int, 10)
	go func() {
		for pct := range progress {
			log.Info().Int("progress", pct).Msg("Erasing")
		}
	}()

	req := &flash.EraseRequest{
		Port:     eraseOpts.port,
		Address:  address,
		Size:     size,
		EraseAll: eraseAll,
		Progress: progress,
	}

	if eraseAll {
		log.Info().Str("port", eraseOpts.port).Msg("Erasing entire flash...")
	} else {
		log.Info().
			Str("port", eraseOpts.port).
			Uint32("address", address).
			Uint32("size", size).
			Msg("Erasing flash region...")
	}

	start := time.Now()
	result := flasher.EraseFlash(context.Background(), req)
	close(progress)
	duration := time.Since(start)

	if !result.Success {
		return fmt.Errorf("erase failed: %w", result.Error)
	}

	log.Info().
		Str("duration", duration.String()).
		Int("bytes", result.Bytes).
		Msg("Erase complete")

	return nil
}

func parseHex(s string) (uint32, error) {
	var value uint64
	_, err := fmt.Sscanf(s, "%x", &value)
	if err != nil {
		return 0, fmt.Errorf("parse hex: %w", err)
	}
	if value > 0xFFFFFFFF {
		return 0, fmt.Errorf("value too large: %s", s)
	}
	return uint32(value), nil
}
