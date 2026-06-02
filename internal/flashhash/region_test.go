package flashhash

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlashRegionInfo_Validate(t *testing.T) {
	tests := []struct {
		name    string
		region  FlashRegionInfo
		wantErr bool
	}{
		{
			name: "Valid region",
			region: FlashRegionInfo{
				Name:   "bootloader",
				Offset: 0x1000,
				Size:   0x7000,
				MD5:    "0123456789abcdef0123456789abcdef",
			},
			wantErr: false,
		},
		{
			name: "Empty name",
			region: FlashRegionInfo{
				Name:   "",
				Offset: 0x1000,
				Size:   0x7000,
				MD5:    "0123456789abcdef0123456789abcdef",
			},
			wantErr: true,
		},
		{
			name: "Zero size",
			region: FlashRegionInfo{
				Name:   "bootloader",
				Offset: 0x1000,
				Size:   0,
				MD5:    "0123456789abcdef0123456789abcdef",
			},
			wantErr: true,
		},
		{
			name: "Empty MD5",
			region: FlashRegionInfo{
				Name:   "bootloader",
				Offset: 0x1000,
				Size:   0x7000,
				MD5:    "",
			},
			wantErr: true,
		},
		{
			name: "Invalid MD5 length",
			region: FlashRegionInfo{
				Name:   "bootloader",
				Offset: 0x1000,
				Size:   0x7000,
				MD5:    "0123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.region.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsStandardRegion(t *testing.T) {
	tests := []struct {
		name     string
		region   string
		expected bool
	}{
		{"Bootloader", RegionBootloader, true},
		{"Partition table", RegionPartitionTable, true},
		{"OTA select", RegionOTASelect, true},
		{"Application", RegionApplication, true},
		{"NVS", RegionNVS, true},
		{"PHY init", RegionPHYInit, true},
		{"Custom", "custom-region", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsStandardRegion(tt.region)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStandardLayouts(t *testing.T) {
	// Test ESP32-S3 layout
	s3Layout := StandardESP32S3Layout4MB()
	assert.Len(t, s3Layout, 4)

	// Check bootloader
	bootloader := s3Layout[0]
	assert.Equal(t, RegionBootloader, bootloader.Name)
	assert.Equal(t, uint32(0x1000), bootloader.Offset)
	assert.Equal(t, uint32(0x7000), bootloader.Size)

	// Check application
	app := s3Layout[3]
	assert.Equal(t, RegionApplication, app.Name)
	assert.Equal(t, uint32(0x10000), app.Offset)
	assert.Equal(t, uint32(0x400000), app.Size)
}

func TestComputeRegionMD5Validation(t *testing.T) {
	// Create test data
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}

	// Compute hash for first 128 bytes
	hash, err := ComputeRegionMD5(data, 0, 128)
	require.NoError(t, err)
	assert.Len(t, hash, 32)

	// Verify it's a valid hex string
	err = ValidateMD5Format(hash)
	assert.NoError(t, err)

	// Test that same data produces same hash
	hash2, err := ComputeRegionMD5(data, 0, 128)
	require.NoError(t, err)
	assert.Equal(t, hash, hash2)

	// Test that different data produces different hash
	hash3, err := ComputeRegionMD5(data, 128, 128)
	require.NoError(t, err)
	assert.NotEqual(t, hash, hash3)
}

func TestValidateMD5Format(t *testing.T) {
	tests := []struct {
		name    string
		hash    string
		wantErr bool
	}{
		{"Valid MD5", "0123456789abcdef0123456789abcdef", false},
		{"Valid MD5 uppercase", "0123456789ABCDEF0123456789ABCDEF", false},
		{"Too short", "0123", true},
		{"Too long", "0123456789abcdef0123456789abcdef00", true},
		{"Invalid character", "0123456789abcdef0123456789abcdegg", true},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMD5Format(tt.hash)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCompareRegions(t *testing.T) {
	// Create client regions with matching hashes
	clientRegions := []FlashRegionInfo{
		{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
		{Name: "partition-table", Offset: 0x8000, Size: 0x1000, MD5: "22222222222222222222222222222222"},
		{Name: "application", Offset: 0x10000, Size: 0x100000, MD5: "33333333333333333333333333333333"},
	}

	// Job regions with different application hash
	jobRegions := []FlashRegionInfo{
		{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
		{Name: "partition-table", Offset: 0x8000, Size: 0x1000, MD5: "22222222222222222222222222222222"},
		{Name: "application", Offset: 0x10000, Size: 0x100000, MD5: "44444444444444444444444444444444"},
	}

	needed, cached := CompareRegions(clientRegions, jobRegions)

	// Should have bootloader and partition table cached (matching)
	assert.Len(t, cached, 2)

	// Should have application needed (mismatch)
	assert.Len(t, needed, 1)
	assert.Equal(t, "application", needed[0].Name)
}

func TestMergeRegions(t *testing.T) {
	clientRegions := []FlashRegionInfo{
		{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
	}

	jobRegions := []FlashRegionInfo{
		{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
		{Name: "partition-table", Offset: 0x8000, Size: 0x1000, MD5: "22222222222222222222222222222222"},
		{Name: "application", Offset: 0x10000, Size: 0x100000, MD5: "33333333333333333333333333333333"},
	}

	missing := MergeRegions(clientRegions, jobRegions)

	// Should have partition-table and application (not in client)
	assert.Len(t, missing, 2)
	assert.Equal(t, "partition-table", missing[0].Name)
	assert.Equal(t, "application", missing[1].Name)
}
