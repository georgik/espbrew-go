package flashhash

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// ComputeRegionMD5 computes the MD5 hash of a specific region within a binary file
func ComputeRegionMD5(data []byte, offset, size uint32) (string, error) {
	if offset+size > uint32(len(data)) {
		return "", fmt.Errorf("region offset+size exceeds data length")
	}
	if size == 0 {
		return "", errors.New("region size cannot be zero")
	}

	regionData := data[offset : offset+size]
	hash := md5.Sum(regionData)
	return hex.EncodeToString(hash[:]), nil
}

// ComputeRegionMD5FromFile computes the MD5 hash of a region from a file
func ComputeRegionMD5FromFile(path string, offset, size uint32) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// Seek to the region offset
	if _, err := file.Seek(int64(offset), io.SeekStart); err != nil {
		return "", fmt.Errorf("seek to offset: %w", err)
	}

	// Read the region data
	regionData := make([]byte, size)
	n, err := io.ReadFull(file, regionData)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("read region data: %w", err)
	}

	// If we read less than expected (EOF), adjust size
	actualSize := uint32(n)
	if actualSize < size {
		regionData = regionData[:actualSize]
	}

	hash := md5.Sum(regionData)
	return hex.EncodeToString(hash[:]), nil
}

// ComputeAllRegionsMD5 computes MD5 hashes for all regions in a binary
func ComputeAllRegionsMD5(data []byte, regions []FlashRegionInfo) ([]FlashRegionInfo, error) {
	result := make([]FlashRegionInfo, len(regions))

	for i, region := range regions {
		hash, err := ComputeRegionMD5(data, region.Offset, region.Size)
		if err != nil {
			return nil, fmt.Errorf("compute hash for region %s: %w", region.Name, err)
		}
		result[i] = FlashRegionInfo{
			Name:   region.Name,
			Offset: region.Offset,
			Size:   region.Size,
			MD5:    hash,
		}
	}

	return result, nil
}

// ComputeAllRegionsMD5FromFile computes MD5 hashes for all regions from a file
func ComputeAllRegionsMD5FromFile(path string, regions []FlashRegionInfo) ([]FlashRegionInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return ComputeAllRegionsMD5(data, regions)
}

// CompareRegions compares client regions with job regions and returns mismatched regions
func CompareRegions(clientRegions []FlashRegionInfo, jobRegions []FlashRegionInfo) (needed, cached []FlashRegionInfo) {
	// Build a map of job regions by name for quick lookup
	jobMap := make(map[string]FlashRegionInfo)
	for _, r := range jobRegions {
		jobMap[r.Name] = r
	}

	for _, client := range clientRegions {
		job, exists := jobMap[client.Name]
		if !exists {
			// Region not in job, skip it
			continue
		}

		// Check if hashes match
		if client.MD5 == job.MD5 {
			cached = append(cached, client)
		} else {
			// Hash mismatch, need to flash this region
			needed = append(needed, job)
		}
	}

	return needed, cached
}

// MergeRegions adds job regions that are not present in client regions
func MergeRegions(clientRegions []FlashRegionInfo, jobRegions []FlashRegionInfo) []FlashRegionInfo {
	result := make([]FlashRegionInfo, 0, len(jobRegions))
	clientMap := make(map[string]bool)

	for _, r := range clientRegions {
		clientMap[r.Name] = true
	}

	for _, job := range jobRegions {
		if !clientMap[job.Name] {
			// Region not on client device, needs to be flashed
			result = append(result, job)
		}
	}

	return result
}

// ValidateMD5Format checks if a string is a valid MD5 hash (32 hex characters)
func ValidateMD5Format(hash string) error {
	if len(hash) != 32 {
		return fmt.Errorf("MD5 hash must be 32 characters, got %d", len(hash))
	}
	for _, c := range hash {
		if !isHexDigit(c) {
			return fmt.Errorf("MD5 hash contains invalid character: %c", c)
		}
	}
	return nil
}

func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
