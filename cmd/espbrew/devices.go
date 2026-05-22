package main

import (
	"encoding/json"
	"fmt"
	"os"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/device"
	"github.com/spf13/cobra"
)

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List connected serial devices",
	RunE:  runDevices,
}

var devicesOpts struct {
	clusterURL string
	json       bool
	espOnly    bool
}

func init() {
	devicesCmd.Flags().StringVar(&devicesOpts.clusterURL, "cluster", "", "Cluster URL for remote devices")
	devicesCmd.Flags().BoolVar(&devicesOpts.json, "json", false, "Output as JSON")
	devicesCmd.Flags().BoolVar(&devicesOpts.espOnly, "esp", false, "Show only ESP devices")

	rootCmd.AddCommand(devicesCmd)
}

func runDevices(cmd *cobra.Command, args []string) error {
	if devicesOpts.clusterURL != "" {
		return runDevicesRemote()
	}
	return runDevicesLocal()
}

func runDevicesLocal() error {
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

func runDevicesRemote() error {
	client := cluster.NewClient(devicesOpts.clusterURL)
	devices, err := client.ListDevices()
	if err != nil {
		return fmt.Errorf("list remote devices: %w", err)
	}

	if devicesOpts.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(devices)
	}

	if len(devices) == 0 {
		fmt.Println("No devices found")
		return nil
	}

	fmt.Printf("Devices from cluster %s:\n", devicesOpts.clusterURL)
	for i, d := range devices {
		fmt.Printf("[%d] %s\n", i, d.Path)
		if d.VID != "" {
			fmt.Printf("    VID: %s, PID: %s\n", d.VID, d.PID)
		}
		if d.NodeID != "" {
			fmt.Printf("    Node: %s\n", d.NodeID)
		}
		fmt.Printf("    State: %s\n", d.State)
	}

	return nil
}
