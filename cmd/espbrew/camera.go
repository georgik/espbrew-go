package main

import (
	"encoding/json"
	"fmt"
	"os"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var cameraCmd = &cobra.Command{
	Use:   "camera",
	Short: "List and manage cameras",
}

var cameraListOpts struct {
	json bool
}

var cameraListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available cameras",
	RunE:  runCameraList,
}

func init() {
	rootCmd.AddCommand(cameraCmd)
	cameraCmd.AddCommand(cameraListCmd)

	cameraListCmd.Flags().BoolVar(&cameraListOpts.json, "json", false, "Output as JSON")
}

func runCameraList(cmd *cobra.Command, args []string) error {
	discoverer := camera.NewDiscoverer()
	cameras, err := discoverer.Discover()
	if err != nil {
		return fmt.Errorf("discover cameras: %w", err)
	}

	if cameraListOpts.json {
		return outputCamerasJSON(cameras)
	}

	return outputCamerasTable(cameras)
}

func outputCamerasTable(cameras []*camera.CameraInfo) error {
	if len(cameras) == 0 {
		log.Info().Msg("No cameras found")
		return nil
	}

	log.Info().Msg("Found cameras:")
	for i, cam := range cameras {
		log.Info().Msgf("  %d. %s", i+1, cam.Name)
		log.Info().Msgf("     ID:     %s", cam.ID)
		log.Info().Msgf("     Backend: %s", cam.Backend)
		if len(cam.Formats) > 0 {
			log.Info().Msgf("     Formats: %d available", len(cam.Formats))
		}
	}

	return nil
}

func outputCamerasJSON(cameras []*camera.CameraInfo) error {
	type JSONCamera struct {
		ID      string               `json:"id"`
		Name    string               `json:"name"`
		Path    string               `json:"path"`
		Backend string               `json:"backend"`
		Formats []camera.VideoFormat `json:"formats"`
	}

	output := make([]JSONCamera, len(cameras))
	for i, cam := range cameras {
		output[i] = JSONCamera{
			ID:      cam.ID,
			Name:    cam.Name,
			Path:    cam.Path,
			Backend: string(cam.Backend),
			Formats: cam.Formats,
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}
