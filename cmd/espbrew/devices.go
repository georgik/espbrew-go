package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/georgik/esp-ci-cluster/internal/device"
	"github.com/spf13/cobra"
)

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List connected serial devices",
	RunE:  runDevices,
}

var devicesOpts struct {
	json    bool
	espOnly bool
}

func init() {
	devicesCmd.Flags().BoolVar(&devicesOpts.json, "json", false, "Output as JSON")
	devicesCmd.Flags().BoolVar(&devicesOpts.espOnly, "esp", false, "Show only ESP devices")

	rootCmd.AddCommand(devicesCmd)
}

func runDevices(cmd *cobra.Command, args []string) error {
	scanner := device.NewScanner()

	var ports []interface{}

	if devicesOpts.espOnly {
		espPorts, err := scanner.ScanESP()
		if err != nil {
			return fmt.Errorf("scan ESP devices: %w", err)
		}
		for _, p := range espPorts {
			ports = append(ports, map[string]interface{}{
				"path": p.Path,
				"vid":  fmt.Sprintf("0x%04x", p.VID),
				"pid":  fmt.Sprintf("0x%04x", p.PID),
				"type": "ESP",
			})
		}
	} else {
		allPorts, err := scanner.Scan()
		if err != nil {
			return fmt.Errorf("scan ports: %w", err)
		}
		for _, p := range allPorts {
			ports = append(ports, map[string]interface{}{
				"path": p.Path,
			})
		}
	}

	if devicesOpts.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(ports)
	}

	if len(ports) == 0 {
		fmt.Println("No devices found")
		return nil
	}

	for i, p := range ports {
		fmt.Printf("[%d] %v\n", i, p)
	}

	return nil
}
