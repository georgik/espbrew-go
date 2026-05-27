package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var capturesCmd = &cobra.Command{
	Use:   "captures",
	Short: "Manage captured images",
	Long:  `List, view, or delete captured images from ~/.espbrew/captures/`,
}

var capturesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List captured images",
	RunE:  runCapturesList,
}

var capturesDeleteCmd = &cobra.Command{
	Use:   "delete [pattern]",
	Short: "Delete captured images",
	Long: `Delete captured images matching a pattern.

The pattern is matched against the full relative path (e.g., "2026-05-27/cam-xxx-123456.jpg").

Use --all to delete all captures (requires confirmation).
Use --older-than to delete captures older than the specified duration.

Examples:
  espbrew captures list
  espbrew captures delete "2026-05-27/cam-*"
  espbrew captures delete --all
  espbrew captures delete --older-than 7d`,
	RunE: runCapturesDelete,
}

var capturesOpts struct {
	all       bool
	olderThan string
	yes       bool
}

func init() {
	rootCmd.AddCommand(capturesCmd)
	capturesCmd.AddCommand(capturesListCmd)
	capturesCmd.AddCommand(capturesDeleteCmd)

	capturesDeleteCmd.Flags().BoolVar(&capturesOpts.all, "all", false, "Delete all captures")
	capturesDeleteCmd.Flags().StringVar(&capturesOpts.olderThan, "older-than", "", "Delete captures older than duration (e.g., 7d, 24h)")
	capturesDeleteCmd.Flags().BoolVar(&capturesOpts.yes, "yes", false, "Skip confirmation")
}

type CaptureFile struct {
	Path     string
	Filename string
	Size     int64
	ModTime  time.Time
}

func getCapturesDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".espbrew", "captures"), nil
}

func listCaptures() ([]CaptureFile, error) {
	capturesDir, err := getCapturesDir()
	if err != nil {
		return nil, err
	}

	var captures []CaptureFile

	err = filepath.Walk(capturesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil
		}

		relPath, err := filepath.Rel(capturesDir, path)
		if err != nil {
			return nil
		}

		captures = append(captures, CaptureFile{
			Path:     relPath,
			Filename: filepath.Base(path),
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		})

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	sort.Slice(captures, func(i, j int) bool {
		return captures[i].ModTime.After(captures[j].ModTime)
	})

	return captures, nil
}

func runCapturesList(cmd *cobra.Command, args []string) error {
	captures, err := listCaptures()
	if err != nil {
		return err
	}

	if len(captures) == 0 {
		log.Info().Msg("No captures found")
		return nil
	}

	log.Info().Msgf("Found %d captures:", len(captures))
	for i, cap := range captures {
		log.Info().Msgf("  %d. %s", i+1, cap.Path)
		log.Info().Msgf("     Size: %s, Modified: %s",
			formatBytes(cap.Size),
			cap.ModTime.Format("2006-01-02 15:04:05"))
	}

	return nil
}

func runCapturesDelete(cmd *cobra.Command, args []string) error {
	capturesDir, err := getCapturesDir()
	if err != nil {
		return err
	}

	captures, err := listCaptures()
	if err != nil {
		return err
	}

	if len(captures) == 0 {
		log.Info().Msg("No captures found")
		return nil
	}

	var toDelete []CaptureFile

	switch {
	case capturesOpts.all:
		toDelete = captures
	case capturesOpts.olderThan != "":
		duration, err := time.ParseDuration(capturesOpts.olderThan)
		if err != nil {
			return fmt.Errorf("parse duration: %w", err)
		}
		cutoff := time.Now().Add(-duration)
		for _, cap := range captures {
			if cap.ModTime.Before(cutoff) {
				toDelete = append(toDelete, cap)
			}
		}
	case len(args) > 0:
		pattern := args[0]
		for _, cap := range captures {
			matched, err := filepath.Match(pattern, cap.Path)
			if err != nil {
				return fmt.Errorf("invalid pattern: %w", err)
			}
			if matched {
				toDelete = append(toDelete, cap)
			}
		}
	default:
		return fmt.Errorf("specify a pattern, --all, or --older-than")
	}

	if len(toDelete) == 0 {
		log.Info().Msg("No captures matched the criteria")
		return nil
	}

	log.Info().Msgf("Will delete %d captures:", len(toDelete))
	for _, cap := range toDelete {
		log.Info().Msgf("  - %s", cap.Path)
	}

	if !capturesOpts.yes {
		fmt.Print("Continue? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			log.Info().Msg("Cancelled")
			return nil
		}
	}

	for _, cap := range toDelete {
		fullPath := filepath.Join(capturesDir, cap.Path)
		if err := os.Remove(fullPath); err != nil {
			log.Error().Err(err).Str("path", cap.Path).Msg("Failed to delete")
		} else {
			log.Info().Str("path", cap.Path).Msg("Deleted")
		}
	}

	log.Info().Msgf("Deleted %d captures", len(toDelete))
	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
