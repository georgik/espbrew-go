package testutil

import (
	"os"
	"path/filepath"
)

// TestELFPath returns the path to the test ELF file.
// Set ESPBREW_TEST_ELF environment variable to specify location.
// Default: ../../testdata/test-esp32s3.elf (relative to this package)
func TestELFPath() string {
	if path := os.Getenv("ESPBREW_TEST_ELF"); path != "" {
		return path
	}
	// Relative path from internal/flash/testutil/ to testdata/
	return filepath.Join("..", "..", "testdata", "test-esp32s3.elf")
}

// SkipIfNoELF skips the test if the ELF file is not found.
// Usage: defer testutil.SkipIfNoELF(t)
func SkipIfNoELF(t testingT) {
	if _, err := os.Stat(TestELFPath()); err != nil {
		t.Skip("Test ELF not found. Set ESPBREW_TEST_ELF environment variable.")
	}
}

type testingT interface {
	Skip(args ...interface{})
}

// TestImagePath returns the path to a test image file.
// Set ESPBREW_TEST_IMAGE_DIR environment variable to specify directory.
func TestImagePath(name string) string {
	if dir := os.Getenv("ESPBREW_TEST_IMAGE_DIR"); dir != "" {
		return filepath.Join(dir, name)
	}
	return filepath.Join("..", "..", "testdata", name)
}
