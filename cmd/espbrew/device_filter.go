package main

import (
	"fmt"
	"strings"

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

// selectClusterDevice resolves a single device from a cluster device list using
// the shared filters plus an optional explicit --device selector. It is the
// cluster-side counterpart of resolveSnapDevice: instead of reading the board
// from the client's local inventory it asks the cluster (the single source of
// truth), so `snap --cluster --device/--filter-alias <alias>` works for a board
// that lives on a remote node. It returns the first `available` match, falling
// back to the first match when none is available.
func selectClusterDevice(devices []cluster.DeviceInfo, boardModel, chipType, alias string, tags []string, deviceID string) (cluster.DeviceInfo, error) {
	filtered, err := filterDevices(devices, boardModel, chipType, alias, tags)
	if err != nil {
		return cluster.DeviceInfo{}, err
	}

	// Apply an explicit --device selector (matched by device ID, alias, or
	// path suffix) on top of the shared filters.
	if deviceID != "" {
		var byID []cluster.DeviceInfo
		for _, d := range filtered {
			if deviceMatchesSelector(d, deviceID) {
				byID = append(byID, d)
			}
		}
		if len(byID) == 0 {
			return cluster.DeviceInfo{}, fmt.Errorf("no device matches %s", deviceID)
		}
		filtered = byID
	}

	// Prefer an available device; fall back to the first match.
	for _, d := range filtered {
		if d.State == "available" {
			return d, nil
		}
	}
	if len(filtered) > 0 {
		return filtered[0], nil
	}
	return cluster.DeviceInfo{}, fmt.Errorf("no devices available on cluster")
}

// deviceMatchesSelector reports whether a cluster device matches an explicit
// --device selector: by device ID, by alias membership, or by path (full path,
// /dev/<base>, or bare base name).
func deviceMatchesSelector(d cluster.DeviceInfo, selector string) bool {
	if selector == "" {
		return false
	}
	if d.DeviceID == selector {
		return true
	}
	for _, a := range d.Aliases {
		if a == selector {
			return true
		}
	}
	if d.Path == selector || d.Path == "/dev/"+selector || strings.HasSuffix(d.Path, "/"+selector) {
		return true
	}
	return false
}
