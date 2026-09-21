package main

import (
	"fmt"

	"github.com/georgik/espbrew-go/internal/cluster"
	"github.com/rs/zerolog/log"
)

// filterDevices filters devices by the provided selection criteria. Any of the
// criteria may be empty (board model, chip type, alias, tags), in which case
// that criterion is ignored. When no criterion is set, every device is returned
// unchanged so the caller can fall back to first-available selection.
//
// This helper is shared by `flash` and `monitor` so both commands resolve a
// device by alias (or chip/board/tags) identically.
func filterDevices(devices []cluster.DeviceInfo, boardModel, chipType, alias string, tags []string) ([]cluster.DeviceInfo, error) {
	// If no filters specified, return all devices
	if boardModel == "" && len(tags) == 0 && chipType == "" && alias == "" {
		return devices, nil
	}

	log.Info().
		Str("board", boardModel).
		Strs("tags", tags).
		Str("chip", chipType).
		Str("alias", alias).
		Msg("Filtering devices")

	// Filter devices by matching against API-provided metadata
	var filtered []cluster.DeviceInfo
	for _, d := range devices {
		if deviceMatchesFilters(d, boardModel, chipType, alias, tags) {
			filtered = append(filtered, d)
			log.Info().Str("device", d.Path).
				Str("board", d.BoardModel).
				Strs("tags", d.Tags).
				Strs("aliases", d.Aliases).
				Msg("Device matches filter")
		}
	}

	log.Info().Int("total", len(devices)).Int("matched", len(filtered)).Msg("Device filter results")

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no devices match the specified filters")
	}

	return filtered, nil
}

// deviceMatchesFilters reports whether a device from the API matches all of the
// provided selection criteria.
func deviceMatchesFilters(d cluster.DeviceInfo, boardModel, chipType, alias string, tags []string) bool {
	// Check board model filter
	if boardModel != "" && d.BoardModel != boardModel {
		return false
	}

	// Check chip type filter
	if chipType != "" && d.ChipType != chipType {
		return false
	}

	// Check alias filter (device must have the specified alias)
	if alias != "" {
		found := false
		for _, a := range d.Aliases {
			if a == alias {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check tags filter (all specified tags must be present)
	for _, requiredTag := range tags {
		found := false
		for _, deviceTag := range d.Tags {
			if deviceTag == requiredTag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
