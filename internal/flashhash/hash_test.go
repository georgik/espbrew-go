package flashhash

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeRegionMD5(t *testing.T) {
	// Create test data
	data := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20,
	}

	// Compute hash for first 16 bytes
	hash, err := ComputeRegionMD5(data, 0, 16)
	require.NoError(t, err)
	assert.Len(t, hash, 32)

	// Verify deterministic result by computing twice
	hash2, err := ComputeRegionMD5(data, 0, 16)
	require.NoError(t, err)
	assert.Equal(t, hash, hash2)

	// Compute hash for second 16 bytes
	hash3, err := ComputeRegionMD5(data, 16, 16)
	require.NoError(t, err)
	assert.NotEqual(t, hash, hash3)

	// Verify different data produces different hash
	data2 := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	hash4, err := ComputeRegionMD5(data2, 0, 4)
	require.NoError(t, err)
	assert.NotEqual(t, hash, hash4)
}

func TestComputeRegionMD5Errors(t *testing.T) {
	data := make([]byte, 100)

	// Test offset + size exceeds data length
	_, err := ComputeRegionMD5(data, 50, 60)
	assert.Error(t, err)

	// Test zero size
	_, err = ComputeRegionMD5(data, 0, 0)
	assert.Error(t, err)

	// Test offset at boundary
	_, err = ComputeRegionMD5(data, 100, 1)
	assert.Error(t, err)
}

func TestComputeAllRegionsMD5(t *testing.T) {
	// Create test data
	data := make([]byte, 0x5000)
	for i := range data {
		data[i] = byte(i % 256)
	}

	regions := []FlashRegionInfo{
		{Name: "region1", Offset: 0x1000, Size: 0x1000},
		{Name: "region2", Offset: 0x2000, Size: 0x1000},
		{Name: "region3", Offset: 0x3000, Size: 0x1000},
	}

	result, err := ComputeAllRegionsMD5(data, regions)
	require.NoError(t, err)
	assert.Len(t, result, 3)

	// Check each region has a hash
	for _, r := range result {
		assert.NotEmpty(t, r.MD5)
		assert.Len(t, r.MD5, 32)
	}

	// Verify deterministic results
	result2, err := ComputeAllRegionsMD5(data, regions)
	require.NoError(t, err)
	for i := range result {
		assert.Equal(t, result[i].MD5, result2[i].MD5)
	}
}

func TestComputeRegionMD5FromFile(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")

	// Write test data
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}
	err := os.WriteFile(testFile, data, 0644)
	require.NoError(t, err)

	// Compute hash from file
	hash, err := ComputeRegionMD5FromFile(testFile, 0, 4096)
	require.NoError(t, err)
	assert.Len(t, hash, 32)

	// Compute hash from memory for comparison
	expectedHash, err := ComputeRegionMD5(data, 0, 4096)
	require.NoError(t, err)

	assert.Equal(t, expectedHash, hash)

	// Test with offset
	hash2, err := ComputeRegionMD5FromFile(testFile, 1024, 2048)
	require.NoError(t, err)

	expectedHash2, err := ComputeRegionMD5(data, 1024, 2048)
	require.NoError(t, err)

	assert.Equal(t, expectedHash2, hash2)
}

func TestComputeRegionMD5FromFileErrors(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")

	// File doesn't exist
	_, err := ComputeRegionMD5FromFile(testFile, 0, 100)
	assert.Error(t, err)

	// Create file with limited data
	data := make([]byte, 100)
	err = os.WriteFile(testFile, data, 0644)
	require.NoError(t, err)

	// Request beyond file size should return partial hash
	hash, err := ComputeRegionMD5FromFile(testFile, 50, 100)
	// This should succeed but with partial data
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputeAllRegionsMD5FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")

	// Write test data (simulating a firmware image)
	// Need enough data for all regions
	data := make([]byte, 0x20000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	err := os.WriteFile(testFile, data, 0644)
	require.NoError(t, err)

	regions := []FlashRegionInfo{
		{Name: "bootloader", Offset: 0x1000, Size: 0x7000},
		{Name: "partition-table", Offset: 0x8000, Size: 0x1000},
		{Name: "application", Offset: 0x10000, Size: 0x8000},
	}

	result, err := ComputeAllRegionsMD5FromFile(testFile, regions)
	require.NoError(t, err)
	assert.Len(t, result, 3)

	// Verify each region has a valid hash
	for _, r := range result {
		assert.NotEmpty(t, r.MD5)
		assert.Len(t, r.MD5, 32)
	}
}
