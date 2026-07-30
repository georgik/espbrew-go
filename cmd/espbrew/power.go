//go:build linux
// +build linux

package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/powercontrol"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var powerCmd = &cobra.Command{
	Use:   "power",
	Short: "Control USB hub power for cold boot",
	Long: `Control USB hub power for device power cycling.

This command allows turning USB ports on/off for devices connected
to supported USB hubs (e.g., Rosonway RSH-A10).

Requires Linux kernel >= 6.0 for sysfs power control interface.`,
}

var powerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show hub and port status",
	RunE:  runPowerStatus,
}

var powerOnCmd = &cobra.Command{
	Use:   "on <port> [port...]",
	Short: "Power on port(s)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runPowerOn,
}

var powerOffCmd = &cobra.Command{
	Use:   "off <port> [port...]",
	Short: "Power off port(s)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runPowerOff,
}

var powerCycleCmd = &cobra.Command{
	Use:   "cycle <port> [--delay <duration>]",
	Short: "Power cycle a port (off -> wait -> on)",
	Args:  cobra.ExactArgs(1),
	RunE:  runPowerCycle,
}

var powerAutoDetectCmd = &cobra.Command{
	Use:   "auto-detect",
	Short: "Auto-detect supported USB hub",
	RunE:  runPowerAutoDetect,
}

type powerFlags struct {
	hubLocation string
	hubVendor   string
	hubProduct  string
	delay       time.Duration
}

var powerOpts powerFlags

func init() {
	rootCmd.AddCommand(powerCmd)
	powerCmd.AddCommand(powerStatusCmd)
	powerCmd.AddCommand(powerOnCmd)
	powerCmd.AddCommand(powerOffCmd)
	powerCmd.AddCommand(powerCycleCmd)
	powerCmd.AddCommand(powerAutoDetectCmd)

	// Common flags
	powerCmd.PersistentFlags().StringVar(&powerOpts.hubLocation, "location", "", "Hub location (e.g., 1-2)")
	powerCmd.PersistentFlags().StringVar(&powerOpts.hubVendor, "vendor", "0bda", "Hub vendor ID")
	powerCmd.PersistentFlags().StringVar(&powerOpts.hubProduct, "product", "0411", "Hub product ID")

	// Cycle-specific flags
	powerCycleCmd.Flags().DurationVar(&powerOpts.delay, "delay", 2*time.Second, "Delay between off and on")
}

func getPowerController() (powercontrol.PowerController, error) {
	ctrl := powercontrol.NewController()
	return ctrl, nil
}

func getHub(ctrl powercontrol.PowerController) (*powercontrol.Hub, error) {
	if powerOpts.hubLocation != "" {
		return ctrl.FindHubByLocation(powerOpts.hubLocation)
	}
	return ctrl.FindHubByVendorProduct(powerOpts.hubVendor, powerOpts.hubProduct)
}

func runPowerStatus(cmd *cobra.Command, args []string) error {
	ctrl, err := getPowerController()
	if err != nil {
		return err
	}

	hubs, err := ctrl.ListHubs()
	if err != nil {
		return fmt.Errorf("list hubs: %w", err)
	}

	if len(hubs) == 0 {
		log.Info().Msg("No supported USB hubs found")
		fmt.Println("Ensure:")
		fmt.Println("  - You're running Linux kernel >= 6.0")
		fmt.Println("  - A supported USB hub is connected")
		fmt.Println("  - You have permissions to access USB devices")
		return nil
	}

	for _, hub := range hubs {
		fmt.Printf("Hub: %s [%s:%s] %d ports\n", hub.Location, hub.Vendor, hub.Product, hub.NumPorts)

		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "Port\tPower\tConnected\tSpeed")

		for port := 1; port <= hub.NumPorts; port++ {
			status, err := ctrl.GetPortStatus(&hub, port)
			if err != nil {
				log.Warn().Int("port", port).Err(err).Msg("Failed to read port status")
				continue
			}

			powerStr := "off"
			if status.Power {
				powerStr = "on"
			}

			connStr := ""
			if status.Connected {
				connStr = "yes"
			}

			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", status.Port, powerStr, connStr, status.Speed)
		}
		tw.Flush()
		fmt.Println()
	}

	return nil
}

func runPowerOn(cmd *cobra.Command, args []string) error {
	ctrl, err := getPowerController()
	if err != nil {
		return err
	}

	hub, err := getHub(ctrl)
	if err != nil {
		return fmt.Errorf("find hub: %w", err)
	}

	for _, arg := range args {
		var port int
		_, err := fmt.Sscanf(arg, "%d", &port)
		if err != nil {
			log.Warn().Str("port", arg).Msg("Invalid port number")
			continue
		}

		log.Info().Int("port", port).Msg("Powering on")
		if err := ctrl.SetPortPower(hub, port, true); err != nil {
			log.Error().Int("port", port).Err(err).Msg("Failed to power on")
			continue
		}
		log.Info().Int("port", port).Msg("Powered on successfully")
	}

	return nil
}

func runPowerOff(cmd *cobra.Command, args []string) error {
	ctrl, err := getPowerController()
	if err != nil {
		return err
	}

	hub, err := getHub(ctrl)
	if err != nil {
		return fmt.Errorf("find hub: %w", err)
	}

	for _, arg := range args {
		var port int
		_, err := fmt.Sscanf(arg, "%d", &port)
		if err != nil {
			log.Warn().Str("port", arg).Msg("Invalid port number")
			continue
		}

		log.Info().Int("port", port).Msg("Powering off")
		if err := ctrl.SetPortPower(hub, port, false); err != nil {
			log.Error().Int("port", port).Err(err).Msg("Failed to power off")
			continue
		}
		log.Info().Int("port", port).Msg("Powered off successfully")
	}

	return nil
}

func runPowerCycle(cmd *cobra.Command, args []string) error {
	ctrl, err := getPowerController()
	if err != nil {
		return err
	}

	hub, err := getHub(ctrl)
	if err != nil {
		return fmt.Errorf("find hub: %w", err)
	}

	var port int
	_, err = fmt.Sscanf(args[0], "%d", &port)
	if err != nil {
		return fmt.Errorf("invalid port number: %w", err)
	}

	log.Info().Int("port", port).Msg("Power cycling")
	fmt.Printf("Port %d: Powering off...\n", port)

	if err := ctrl.SetPortPower(hub, port, false); err != nil {
		return fmt.Errorf("power off: %w", err)
	}

	fmt.Printf("Port %d: Waiting %v...\n", port, powerOpts.delay)
	time.Sleep(powerOpts.delay)

	fmt.Printf("Port %d: Powering on...\n", port)
	if err := ctrl.SetPortPower(hub, port, true); err != nil {
		return fmt.Errorf("power on: %w", err)
	}

	log.Info().Int("port", port).Msg("Power cycle complete")
	fmt.Printf("Port %d: Power cycle complete\n", port)

	return nil
}

func runPowerAutoDetect(cmd *cobra.Command, args []string) error {
	ctrl, err := getPowerController()
	if err != nil {
		return err
	}

	hub, err := ctrl.FindHubByVendorProduct(powerOpts.hubVendor, powerOpts.hubProduct)
	if err != nil {
		if err == powercontrol.ErrHubNotFound {
			log.Info().Msg("No supported hub found")
			fmt.Println("Supported hubs (vendor:product):")
			fmt.Println("  0bda:0411 - Rosonway RSH-A10 (10 ports, USB 3.0)")
			return nil
		}
		return fmt.Errorf("find hub: %w", err)
	}

	fmt.Printf("Found hub at location: %s\n", hub.Location)
	fmt.Printf("  Vendor:Product: %s:%s\n", hub.Vendor, hub.Product)
	fmt.Printf("  Ports: %d\n", hub.NumPorts)
	if hub.SuperSpeed {
		fmt.Printf("  Speed: USB 3.0\n")
	} else {
		fmt.Printf("  Speed: USB 2.0\n")
	}

	// Show how to use this hub
	fmt.Println("\nTo use this hub, add --location flag:")
	fmt.Printf("  espbrew power status --location %s\n", hub.Location)

	return nil
}
