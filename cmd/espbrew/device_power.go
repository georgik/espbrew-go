package main

import (
	"fmt"
	"os"

	"github.com/georgik/espbrew-go/internal/cluster"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var devicePowerCmd = &cobra.Command{
	Use:   "device-power <alias>",
	Short: "Power a power-managed device up or down via the cluster",
	Long: `Request the cluster leader to switch a power-managed board's hub port on or off.

The board must declare usb_location/usb_port in espbrew.toml (power-managed).
The alias is resolved server-side, so the real /dev path never leaves this host.

Examples:
  # wake the board (power the hub port on)
  espbrew --cluster http://localhost:8080 device-power esp32-c3-lcdkit --on

  # power the board off (put it to sleep)
  espbrew --cluster http://localhost:8080 device-power esp32-c3-lcdkit --off

With no --on/--off the usage is printed (parameter error).`,
	Args: cobra.ExactArgs(1),
	RunE: runDevicePower,
}

var devicePowerOpts struct {
	clusterURL string
	on         bool
	off        bool
}

func init() {
	devicePowerCmd.Flags().StringVar(&devicePowerOpts.clusterURL, "cluster", os.Getenv("ESPBREW_CLUSTER"), "Cluster URL for remote power control")
	devicePowerCmd.Flags().BoolVar(&devicePowerOpts.on, "on", false, "Power the device on (wake)")
	devicePowerCmd.Flags().BoolVar(&devicePowerOpts.off, "off", false, "Power the device off (sleep)")
	rootCmd.AddCommand(devicePowerCmd)
}

func runDevicePower(cmd *cobra.Command, args []string) error {
	alias := args[0]

	on := devicePowerOpts.on
	if devicePowerOpts.off {
		on = false
	}
	if !devicePowerOpts.on && !devicePowerOpts.off {
		// Parameter problem: print this command's usage.
		return usageErrf("specify --on or --off")
	}

	client := cluster.NewClient(devicePowerOpts.clusterURL)
	resp, err := client.PowerDevice(alias, on)
	if err != nil {
		return err
	}

	state := "on"
	if resp.Sleeping {
		state = "sleeping"
	}
	log.Info().
		Str("alias", resp.Device).
		Str("path", resp.Path).
		Bool("power_on", resp.Power).
		Str("state", state).
		Msg("Device power request complete")

	fmt.Printf("%s: powered %s (sleeping=%v)\n", resp.Device, boolToStr(resp.Power), resp.Sleeping)
	return nil
}

func boolToStr(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
