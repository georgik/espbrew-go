package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"codeberg.org/georgik/espbrew-go/internal/inventory"
	"codeberg.org/georgik/espbrew-go/internal/inventory/rom"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var deviceCmd = &cobra.Command{
	Use:   "device",
	Short: "Manage device inventory",
	Long:  `Manage device inventory for tracking ESP devices by their unique MAC addresses.`,
}

var deviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all devices in inventory",
	RunE:  runDeviceList,
}

var deviceShowCmd = &cobra.Command{
	Use:   "show <device-id|alias|mac>",
	Short: "Show device details",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeviceShow,
}

var deviceProbeCmd = &cobra.Command{
	Use:   "probe <port>",
	Short: "Probe a device and show its identity",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeviceProbe,
}

var deviceDeleteCmd = &cobra.Command{
	Use:   "delete <device-id>",
	Short: "Delete a device from inventory",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeviceDelete,
}

var deviceAliasCmd = &cobra.Command{
	Use:   "alias <device-id>",
	Short: "Manage device aliases",
	Args:  cobra.ExactArgs(1),
}

var deviceTagCmd = &cobra.Command{
	Use:   "tag <device-id>",
	Short: "Manage device tags",
	Args:  cobra.ExactArgs(1),
}

var deviceSetCmd = &cobra.Command{
	Use:   "set <device-id>",
	Short: "Set device properties",
	Args:  cobra.ExactArgs(1),
}

type deviceFlags struct {
	aliasAdd    []string
	aliasRemove []string
	tagAdd      []string
	tagRemove   []string
	tagClear    bool
	boardModel  string
	description string
}

var deviceOpts deviceFlags

func init() {
	rootCmd.AddCommand(deviceCmd)
	deviceCmd.AddCommand(deviceListCmd)
	deviceCmd.AddCommand(deviceShowCmd)
	deviceCmd.AddCommand(deviceProbeCmd)
	deviceCmd.AddCommand(deviceDeleteCmd)
	deviceCmd.AddCommand(deviceAliasCmd)
	deviceCmd.AddCommand(deviceTagCmd)
	deviceCmd.AddCommand(deviceSetCmd)

	// Alias flags
	deviceAliasCmd.Flags().StringSliceVar(&deviceOpts.aliasAdd, "add", nil, "Add alias")
	deviceAliasCmd.Flags().StringSliceVar(&deviceOpts.aliasRemove, "remove", nil, "Remove alias")

	// Tag flags
	deviceTagCmd.Flags().StringSliceVar(&deviceOpts.tagAdd, "add", nil, "Add tag")
	deviceTagCmd.Flags().StringSliceVar(&deviceOpts.tagRemove, "remove", nil, "Remove tag")
	deviceTagCmd.Flags().BoolVar(&deviceOpts.tagClear, "clear", false, "Clear all tags")

	// Set flags
	deviceSetCmd.Flags().StringVar(&deviceOpts.boardModel, "model", "", "Set board model")
	deviceSetCmd.Flags().StringVar(&deviceOpts.description, "description", "", "Set description")
}

func runDeviceList(cmd *cobra.Command, args []string) error {
	inv, err := inventory.NewInventory()
	if err != nil {
		return err
	}

	devices := inv.List()
	if len(devices) == 0 {
		log.Info().Msg("No devices in inventory")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DEVICE ID\tCHIP\tREVISION\tPSRAM\tFLASH\tALIASES\tTAGS")
	for _, dev := range devices {
		psram := formatBytes(int64(dev.PSRAMSize))
		if dev.PSRAMSize == 0 {
			psram = "-"
		}
		flash := formatBytes(int64(dev.FlashSize))
		if dev.FlashSize == 0 {
			flash = "-"
		}
		aliases := joinLimit(dev.Aliases, ",", 2)
		tags := joinLimit(dev.Tags, ",", 3)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			dev.DeviceID, dev.ChipType, dev.ChipRev, psram, flash, aliases, tags)
	}
	tw.Flush()

	return nil
}

func runDeviceShow(cmd *cobra.Command, args []string) error {
	inv, err := inventory.NewInventory()
	if err != nil {
		return err
	}

	// Try to find by device ID, alias, or MAC
	dev, err := findDevice(inv, args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Device ID:    %s\n", dev.DeviceID)
	fmt.Printf("MAC Address:  %s\n", dev.MACAddress)
	fmt.Printf("Chip Type:    %s\n", dev.ChipType)
	fmt.Printf("Revision:     %s\n", dev.ChipRev)
	fmt.Printf("Flash Size:   %s\n", formatBytes(int64(dev.FlashSize)))
	fmt.Printf("PSRAM Size:   %s", formatBytes(int64(dev.PSRAMSize)))
	if dev.PSRAMType != "" {
		fmt.Printf(" (%s)", dev.PSRAMType)
	}
	fmt.Println()
	if dev.BoardModel != "" {
		fmt.Printf("Board Model:  %s\n", dev.BoardModel)
	}
	if dev.Description != "" {
		fmt.Printf("Description:  %s\n", dev.Description)
	}
	fmt.Printf("First Seen:   %s\n", dev.FirstSeen.Format("2006-01-02 15:04:05"))
	fmt.Printf("Last Seen:    %s\n", dev.LastSeen.Format("2006-01-02 15:04:05"))
	fmt.Printf("Last Path:    %s\n", dev.LastPath)
	if dev.NodeID != "" {
		fmt.Printf("Node ID:      %s\n", dev.NodeID)
	}
	if len(dev.Aliases) > 0 {
		fmt.Printf("Aliases:      %s\n", joinLimit(dev.Aliases, ", ", 10))
	}
	if len(dev.Tags) > 0 {
		fmt.Printf("Tags:         %s\n", joinLimit(dev.Tags, ", ", 10))
	}

	return nil
}

func runDeviceProbe(cmd *cobra.Command, args []string) error {
	port := args[0]

	log.Info().Str("port", port).Msg("Probing device")

	identity, err := inventory.ProbeDevice(port)
	if err != nil {
		return fmt.Errorf("probe failed: %w", err)
	}

	log.Info().Str("chip", identity.Chip).Msg("Chip detected")
	fmt.Printf("MAC Address:  %s\n", identity.MAC)
	fmt.Printf("Chip Type:    %s\n", identity.Chip)
	fmt.Printf("Revision:     %d.%d\n", identity.ChipMajor, identity.ChipMinor)
	if identity.FlashSize > 0 {
		fmt.Printf("Flash Size:   %s\n", formatBytes(int64(identity.FlashSize)))
	}
	if identity.PSRAMSize > 0 {
		fmt.Printf("PSRAM Size:   %s", formatBytes(int64(identity.PSRAMSize)))
		if identity.PSRAMType != "" {
			fmt.Printf(" (%s)", identity.PSRAMType)
		}
		fmt.Println()
	}

	// Show device ID
	deviceID := rom.DeviceID(identity.MAC)
	fmt.Printf("\nDevice ID:     %s\n", deviceID)

	// Offer to add to inventory
	inv, err := inventory.NewInventory()
	if err != nil {
		return err
	}

	// Check if already exists
	existing, err := inv.Get(deviceID)
	if err == nil {
		log.Info().Str("device_id", deviceID).Msg("Device already in inventory")
		fmt.Printf("Last seen:     %s\n", existing.LastSeen.Format("2006-01-02 15:04:05"))
		return nil
	}

	// Add new device
	_, err = inv.GetOrCreate(identity, port, "")
	if err != nil {
		return fmt.Errorf("add to inventory: %w", err)
	}

	log.Info().Str("device_id", deviceID).Msg("Device added to inventory")
	return nil
}

func runDeviceDelete(cmd *cobra.Command, args []string) error {
	inv, err := inventory.NewInventory()
	if err != nil {
		return err
	}

	deviceID := args[0]

	// Confirm deletion
	fmt.Printf("Delete device %s? (y/N): ", deviceID)
	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		log.Info().Msg("Cancelled")
		return nil
	}

	if err := inv.Delete(deviceID); err != nil {
		return err
	}

	log.Info().Str("device_id", deviceID).Msg("Device deleted")
	return nil
}

func runDeviceAlias(cmd *cobra.Command, args []string) error {
	inv, err := inventory.NewInventory()
	if err != nil {
		return err
	}

	dev, err := findDevice(inv, args[0])
	if err != nil {
		return err
	}

	// Add aliases
	for _, alias := range deviceOpts.aliasAdd {
		if err := inv.AddAlias(dev.DeviceID, alias); err != nil {
			return err
		}
		log.Info().Str("device_id", dev.DeviceID).Str("alias", alias).Msg("Alias added")
	}

	// Remove aliases
	for _, alias := range deviceOpts.aliasRemove {
		if err := inv.RemoveAlias(dev.DeviceID, alias); err != nil {
			return err
		}
		log.Info().Str("device_id", dev.DeviceID).Str("alias", alias).Msg("Alias removed")
	}

	if len(deviceOpts.aliasAdd) == 0 && len(deviceOpts.aliasRemove) == 0 {
		// Show current aliases
		if len(dev.Aliases) > 0 {
			for _, alias := range dev.Aliases {
				fmt.Println(alias)
			}
		} else {
			log.Info().Msg("No aliases set")
		}
	}

	return nil
}

func runDeviceTag(cmd *cobra.Command, args []string) error {
	inv, err := inventory.NewInventory()
	if err != nil {
		return err
	}

	dev, err := findDevice(inv, args[0])
	if err != nil {
		return err
	}

	// Clear all tags
	if deviceOpts.tagClear {
		if err := inv.UpdateTags(dev.DeviceID, []string{}); err != nil {
			return err
		}
		log.Info().Str("device_id", dev.DeviceID).Msg("All tags cleared")
		return nil
	}

	// Add tags
	for _, tag := range deviceOpts.tagAdd {
		if err := inv.AddTag(dev.DeviceID, tag); err != nil {
			return err
		}
		log.Info().Str("device_id", dev.DeviceID).Str("tag", tag).Msg("Tag added")
	}

	// Remove tags
	for _, tag := range deviceOpts.tagRemove {
		if err := inv.RemoveTag(dev.DeviceID, tag); err != nil {
			return err
		}
		log.Info().Str("device_id", dev.DeviceID).Str("tag", tag).Msg("Tag removed")
	}

	if len(deviceOpts.tagAdd) == 0 && len(deviceOpts.tagRemove) == 0 && !deviceOpts.tagClear {
		// Show current tags
		if len(dev.Tags) > 0 {
			for _, tag := range dev.Tags {
				fmt.Println(tag)
			}
		} else {
			log.Info().Msg("No tags set")
		}
	}

	return nil
}

func runDeviceSet(cmd *cobra.Command, args []string) error {
	inv, err := inventory.NewInventory()
	if err != nil {
		return err
	}

	dev, err := findDevice(inv, args[0])
	if err != nil {
		return err
	}

	if deviceOpts.boardModel != "" {
		if err := inv.SetBoardModel(dev.DeviceID, deviceOpts.boardModel); err != nil {
			return err
		}
		log.Info().Str("device_id", dev.DeviceID).Str("model", deviceOpts.boardModel).Msg("Board model set")
	}

	if deviceOpts.description != "" {
		if err := inv.SetDescription(dev.DeviceID, deviceOpts.description); err != nil {
			return err
		}
		log.Info().Str("device_id", dev.DeviceID).Msg("Description set")
	}

	return nil
}

// findDevice finds a device by ID, alias, or MAC address
func findDevice(inv *inventory.Inventory, identifier string) (*inventory.DeviceInventory, error) {
	// Try as device ID first
	dev, err := inv.Get(identifier)
	if err == nil {
		return dev, nil
	}

	// Try as alias
	dev, err = inv.FindByAlias(identifier)
	if err == nil {
		return dev, nil
	}

	// Try as MAC address (add esp- prefix if not present)
	mac := identifier
	if len(mac) == 17 && mac[2] == ':' { // Already formatted MAC
		// Try with esp- prefix
		dev, err = inv.Get("esp-" + mac)
		if err == nil {
			return dev, nil
		}
	}

	return nil, fmt.Errorf("device not found: %s", identifier)
}

func joinLimit(items []string, sep string, limit int) string {
	if len(items) == 0 {
		return "-"
	}
	if len(items) <= limit {
		result := ""
		for i, item := range items {
			if i > 0 {
				result += sep
			}
			result += item
		}
		return result
	}
	result := ""
	for i := 0; i < limit-1; i++ {
		if i > 0 {
			result += sep
		}
		result += items[i]
	}
	return result + sep + fmt.Sprintf("... +%d", len(items)-limit+1)
}
