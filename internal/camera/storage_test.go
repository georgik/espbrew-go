package camera

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if store == nil {
		t.Fatal("NewStore() returned nil")
	}

	if store.GetBaseDir() != tmpDir {
		t.Errorf("GetBaseDir() = %v, want %v", store.GetBaseDir(), tmpDir)
	}

	// Check directory was created
	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Failed to stat store dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("Store base path is not a directory")
	}
}

func TestStoreGetDateDir(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	dateDir, err := store.GetDateDir()
	if err != nil {
		t.Fatalf("GetDateDir() error = %v", err)
	}

	// Check directory exists
	info, err := os.Stat(dateDir)
	if err != nil {
		t.Fatalf("Failed to stat date dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("Date dir is not a directory")
	}

	// Check it's in the base dir
	if !filepath.HasPrefix(dateDir, tmpDir) {
		t.Errorf("Date dir %q is not in base dir %q", dateDir, tmpDir)
	}
}

func TestStoreGenerateFilename(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	cameraID := "test-camera-abc123def456"
	format := "jpg"

	filename, err := store.GenerateFilename(cameraID, format)
	if err != nil {
		t.Fatalf("GenerateFilename() error = %v", err)
	}

	// Check extension
	ext := filepath.Ext(filename)
	if ext != "."+format {
		t.Errorf("Filename extension = %v, want %v", ext, "."+format)
	}

	// Check it contains shortened camera ID (first 8 chars)
	base := filepath.Base(filename)
	if !contains(base, "test-cam") {
		t.Errorf("Filename %q should contain short camera ID 'test-cam', got %q", filename, base)
	}

	// Check it's in the base dir
	if !filepath.HasPrefix(filename, tmpDir) {
		t.Errorf("Filename %q is not in base dir %q", filename, tmpDir)
	}
}

func TestStoreSave(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	cameraID := "cam-test-001"
	format := "jpg"
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG header

	path, err := store.Save(cameraID, format, data)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Check file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Saved file does not exist: %v", err)
	}

	// Check file content
	savedData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if len(savedData) != len(data) {
		t.Errorf("Saved data size = %v, want %v", len(savedData), len(data))
	}

	// Check metadata file exists
	dateDir := filepath.Dir(path)
	metadataPath := filepath.Join(dateDir, "metadata.json")
	if _, err := os.Stat(metadataPath); err != nil {
		t.Errorf("Metadata file does not exist: %v", err)
	}
}

func TestStoreListCaptures(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	cameraID := "cam-list-test"
	format := "jpg"
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0}

	// Save a few captures
	for i := 0; i < 3; i++ {
		_, err := store.Save(cameraID, format, data)
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	// List captures for today
	captures, err := store.ListCaptures(time.Now())
	if err != nil {
		t.Fatalf("ListCaptures() error = %v", err)
	}

	if len(captures) != 3 {
		t.Errorf("ListCaptures() returned %d captures, want 3", len(captures))
	}

	// Check capture metadata
	for _, cap := range captures {
		if cap.CameraID != cameraID {
			t.Errorf("Capture CameraID = %v, want %v", cap.CameraID, cameraID)
		}
		if cap.Format != format {
			t.Errorf("Capture Format = %v, want %v", cap.Format, format)
		}
		if cap.SizeBytes != int64(len(data)) {
			t.Errorf("Capture SizeBytes = %v, want %v", cap.SizeBytes, len(data))
		}
	}
}

func TestStoreCleanupOld(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Create old date directories
	oldDate := time.Now().Add(-48 * time.Hour)
	oldDir := filepath.Join(tmpDir, oldDate.Format("2006-01-02"))
	os.MkdirAll(oldDir, 0755)
	os.WriteFile(filepath.Join(oldDir, "test.jpg"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(oldDir, "metadata.json"), []byte("{}"), 0644)

	// Create new date directory
	newDir, _ := store.GetDateDir()
	os.WriteFile(filepath.Join(newDir, "test.jpg"), []byte("new"), 0644)

	// Cleanup captures older than 24 hours
	err := store.CleanupOld(24 * time.Hour)
	if err != nil {
		t.Fatalf("CleanupOld() error = %v", err)
	}

	// Old directory should be removed
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("Old directory still exists after cleanup")
	}

	// New directory should still exist
	if _, err := os.Stat(newDir); err != nil {
		t.Errorf("New directory does not exist after cleanup: %v", err)
	}
}

func TestDefaultStore(t *testing.T) {
	// This test creates a real store in the user's home directory
	// but only if ESPBREW_TEST_STORAGE is set
	if os.Getenv("ESPBREW_TEST_STORAGE") == "" {
		t.Skip("Set ESPBREW_TEST_STORAGE=1 to test default store")
	}

	store, err := DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expectedDir := filepath.Join(homeDir, ".espbrew", "captures")
	if store.GetBaseDir() != expectedDir {
		t.Errorf("DefaultStore() dir = %v, want %v", store.GetBaseDir(), expectedDir)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
