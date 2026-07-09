package main

import (
	"fmt"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"github.com/spf13/cobra"
)

var camerasCmd = &cobra.Command{
	Use:   "cameras",
	Short: "List available cameras",
	Long: `List all available video cameras connected to the system.

Shows camera name, ID, backend, and device path for each discovered camera.`,
	RunE: runCameras,
}

func init() {
	rootCmd.AddCommand(camerasCmd)
}

func runCameras(cmd *cobra.Command, args []string) error {
	discoverer := camera.NewDiscoverer()
	cameras, err := discoverer.Discover()
	if err != nil {
		return fmt.Errorf("discover cameras: %w", err)
	}

	if len(cameras) == 0 {
		fmt.Println("No cameras found")
		return nil
	}

	fmt.Printf("Found %d camera(s):\n\n", len(cameras))
	for i, cam := range cameras {
		fmt.Printf("%d. %s\n", i+1, cam.Name)
		fmt.Printf("   ID:     %s\n", cam.ID)
		fmt.Printf("   Backend: %s\n", cam.Backend)
		if cam.Path != "" {
			fmt.Printf("   Path:   %s\n", cam.Path)
		}
		fmt.Println()
	}

	return nil
}
