package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create files for testing
func createFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
}

func TestESPIDFDetector_Name(t *testing.T) {
	detector := &ESPIDFDetector{}
	assert.Equal(t, "esp-idf", detector.Name())
}

func TestESPIDFDetector_Detect(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{
			name: "valid ESP-IDF project",
			files: map[string]string{
				"CMakeLists.txt": "cmake_minimum_required(VERSION 3.5)",
				"sdkconfig":      "CONFIG_IDF_TARGET=esp32",
			},
			want: true,
		},
		{
			name: "ESP-IDF with sdkconfig.defaults",
			files: map[string]string{
				"CMakeLists.txt":     "cmake_minimum_required(VERSION 3.5)",
				"sdkconfig.defaults": "CONFIG_IDF_TARGET=esp32",
			},
			want: true,
		},
		{
			name: "missing sdkconfig",
			files: map[string]string{
				"CMakeLists.txt": "cmake_minimum_required(VERSION 3.5)",
			},
			want: false,
		},
		{
			name: "missing CMakeLists.txt",
			files: map[string]string{
				"sdkconfig": "CONFIG_IDF_TARGET=esp32",
			},
			want: false,
		},
		{
			name:  "empty directory",
			files: map[string]string{},
			want:  false,
		},
		{
			name: "with main source file",
			files: map[string]string{
				"CMakeLists.txt":      "cmake_minimum_required(VERSION 3.5)",
				"sdkconfig":           "CONFIG_IDF_TARGET=esp32",
				"main/main.c":         "void app_main() {}",
				"main/CMakeLists.txt": "idf_component_register(SRCS \"main.c\")",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			createFiles(t, tmpDir, tt.files)

			detector := &ESPIDFDetector{}
			got := detector.Detect(tmpDir)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestESPIDFDetector_FindBuildDir(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string) error
		wantErr   bool
		checkPath func(*testing.T, string)
	}{
		{
			name: "build directory exists with build.ninja",
			setup: func(tmpDir string) error {
				buildDir := filepath.Join(tmpDir, "build")
				if err := os.Mkdir(buildDir, 0755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(buildDir, "build.ninja"), []byte("ninja"), 0644)
			},
			wantErr: false,
			checkPath: func(t *testing.T, path string) {
				assert.Contains(t, path, "build")
			},
		},
		{
			name: "build directory exists with Makefile",
			setup: func(tmpDir string) error {
				buildDir := filepath.Join(tmpDir, "build")
				if err := os.Mkdir(buildDir, 0755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(buildDir, "Makefile"), []byte("make"), 0644)
			},
			wantErr: false,
			checkPath: func(t *testing.T, path string) {
				assert.Contains(t, path, "build")
			},
		},
		{
			name: "no build directory",
			setup: func(tmpDir string) error {
				return nil // No build dir
			},
			wantErr: true,
		},
		{
			name: "build directory exists but not a build dir",
			setup: func(tmpDir string) error {
				return os.Mkdir(filepath.Join(tmpDir, "build"), 0755)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatal(err)
			}

			detector := &ESPIDFDetector{}
			buildDir, err := detector.FindBuildDir(tmpDir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, buildDir)
				if tt.checkPath != nil {
					tt.checkPath(t, buildDir)
				}
			}
		})
	}
}

func TestESPIDFDetector_GetArtifacts(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string) error
		want      *BuildArtifacts
		checkFunc func(*testing.T, *BuildArtifacts)
	}{
		{
			name: "all artifacts present",
			setup: func(tmpDir string) error {
				if err := os.MkdirAll(filepath.Join(tmpDir, "bootloader"), 0755); err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Join(tmpDir, "partition_table"), 0755); err != nil {
					return err
				}
				os.WriteFile(filepath.Join(tmpDir, "bootloader", "bootloader.bin"), []byte("boot"), 0644)
				os.WriteFile(filepath.Join(tmpDir, "partition_table", "partition-table.bin"), []byte("part"), 0644)
				os.WriteFile(filepath.Join(tmpDir, "firmware.bin"), []byte("app"), 0644)
				return nil
			},
			checkFunc: func(t *testing.T, a *BuildArtifacts) {
				assert.Contains(t, a.Bootloader, "bootloader.bin")
				assert.Contains(t, a.Partitions, "partition-table.bin")
				assert.Contains(t, a.App, "firmware.bin")
			},
		},
		{
			name: "only app.bin present",
			setup: func(tmpDir string) error {
				return os.WriteFile(filepath.Join(tmpDir, "app.bin"), []byte("app"), 0644)
			},
			checkFunc: func(t *testing.T, a *BuildArtifacts) {
				assert.Empty(t, a.Bootloader)
				assert.Empty(t, a.Partitions)
				assert.Contains(t, a.App, "app.bin")
			},
		},
		{
			name: "largest bin selected as app",
			setup: func(tmpDir string) error {
				os.WriteFile(filepath.Join(tmpDir, "small.bin"), []byte("small"), 0644)
				os.WriteFile(filepath.Join(tmpDir, "large.bin"), []byte("large content here"), 0644)
				os.WriteFile(filepath.Join(tmpDir, "medium.bin"), []byte("medium"), 0644)
				return nil
			},
			checkFunc: func(t *testing.T, a *BuildArtifacts) {
				assert.Contains(t, a.App, "large.bin")
			},
		},
		{
			name:  "empty build directory",
			setup: func(string) error { return nil },
			checkFunc: func(t *testing.T, a *BuildArtifacts) {
				assert.Empty(t, a.Bootloader)
				assert.Empty(t, a.Partitions)
				assert.Empty(t, a.App)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatal(err)
			}

			detector := &ESPIDFDetector{}
			artifacts, err := detector.GetArtifacts(tmpDir)

			require.NoError(t, err)
			assert.Equal(t, tmpDir, artifacts.BuildDir)
			assert.Equal(t, filepath.Join(tmpDir, "flash_args"), artifacts.FlashArgs)
			if tt.checkFunc != nil {
				tt.checkFunc(t, artifacts)
			}
		})
	}
}

// TestESPIDFDetector_GetArtifacts_ExtraPartitions verifies that additional
// partitions listed in flash_args (beyond bootloader/partitions/app) are detected
// and keep their flash offsets. This covers the case of a custom partition table
// that emits extra data images, e.g. a pre-populated FAT storage partition.
func TestESPIDFDetector_GetArtifacts_ExtraPartitions(t *testing.T) {
	tmpDir := t.TempDir()

	// Standard ESP-IDF build layout.
	if err := os.MkdirAll(filepath.Join(tmpDir, "bootloader"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "partition_table"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tmpDir, "bootloader", "bootloader.bin"), []byte("boot"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "partition_table", "partition-table.bin"), []byte("part"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "esp32s3_hello.bin"), []byte("app-image"), 0644)
	// Extra data partition (e.g. fatfs_create_spiflash_image -> storage.bin).
	os.WriteFile(filepath.Join(tmpDir, "storage.bin"), []byte("fat-image"), 0644)

	// flash_args lists all four images with their offsets.
	flashArgs := "--flash-mode dio --flash-freq 80m --flash-size 2MB\n" +
		"0x0 bootloader/bootloader.bin\n" +
		"0x8000 partition_table/partition-table.bin\n" +
		"0x50000 esp32s3_hello.bin\n" +
		"0x10000 storage.bin\n"
	os.WriteFile(filepath.Join(tmpDir, "flash_args"), []byte(flashArgs), 0644)

	d := &ESPIDFDetector{}
	artifacts, err := d.GetArtifacts(tmpDir)
	if err != nil {
		t.Fatalf("GetArtifacts: %v", err)
	}

	// Standard slots still resolved.
	if artifacts.Bootloader == "" || artifacts.Partitions == "" || artifacts.App == "" {
		t.Fatalf("standard slots not fully resolved: %+v", artifacts)
	}

	// Exactly one extra file: storage.bin at 0x10000.
	if len(artifacts.ExtraFiles) != 1 {
		t.Fatalf("expected 1 extra file, got %d: %+v", len(artifacts.ExtraFiles), artifacts.ExtraFiles)
	}
	ef := artifacts.ExtraFiles[0]
	if ef.Offset != 0x10000 {
		t.Fatalf("expected extra offset 0x10000, got 0x%x", ef.Offset)
	}
	if filepath.Base(ef.Path) != "storage.bin" {
		t.Fatalf("expected extra file storage.bin, got %s", ef.Path)
	}
}

// TestESPIDFDetector_GetArtifacts_NoExtraPartitions verifies that a project whose
// flash_args only lists the standard three images yields no extra files.
func TestESPIDFDetector_GetArtifacts_NoExtraPartitions(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "bootloader"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "partition_table"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tmpDir, "bootloader", "bootloader.bin"), []byte("boot"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "partition_table", "partition-table.bin"), []byte("part"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "firmware.bin"), []byte("app"), 0644)

	// flash_args with only the standard three images.
	flashArgs := "--flash-mode dio --flash-freq 80m --flash-size 4MB\n" +
		"0x0 bootloader/bootloader.bin\n" +
		"0x8000 partition_table/partition-table.bin\n" +
		"0x10000 firmware.bin\n"
	os.WriteFile(filepath.Join(tmpDir, "flash_args"), []byte(flashArgs), 0644)

	d := &ESPIDFDetector{}
	artifacts, err := d.GetArtifacts(tmpDir)
	if err != nil {
		t.Fatalf("GetArtifacts: %v", err)
	}

	if len(artifacts.ExtraFiles) != 0 {
		t.Fatalf("expected no extra files, got %d: %+v", len(artifacts.ExtraFiles), artifacts.ExtraFiles)
	}
}

// TestESPIDFDetector_GetArtifacts_ExtraPartitionMissingFile verifies that a
// flash_args entry pointing at a missing file is skipped (not panic/err).
func TestESPIDFDetector_GetArtifacts_ExtraPartitionMissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "bootloader"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "partition_table"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tmpDir, "bootloader", "bootloader.bin"), []byte("boot"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "partition_table", "partition-table.bin"), []byte("part"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "firmware.bin"), []byte("app"), 0644)

	// flash_args references a nonexistent extra.bin.
	flashArgs := "--flash-mode dio --flash-freq 80m --flash-size 4MB\n" +
		"0x0 bootloader/bootloader.bin\n" +
		"0x8000 partition_table/partition-table.bin\n" +
		"0x10000 firmware.bin\n" +
		"0x50000 missing.bin\n"
	os.WriteFile(filepath.Join(tmpDir, "flash_args"), []byte(flashArgs), 0644)

	d := &ESPIDFDetector{}
	artifacts, err := d.GetArtifacts(tmpDir)
	if err != nil {
		t.Fatalf("GetArtifacts: %v", err)
	}

	if len(artifacts.ExtraFiles) != 0 {
		t.Fatalf("expected missing file to be skipped, got %d: %+v", len(artifacts.ExtraFiles), artifacts.ExtraFiles)
	}
}

// TestESPIDFDetector_GetArtifacts_FlashFiles verifies that BuildArtifacts.FlashFiles
// is the authoritative, ordered flash plan parsed from flash_args, with each image
// resolved to an absolute path and its real flash offset. This guards the regression
// where the factory app (0x50000) was flashed at the preset 0x10000 and never booted.
func TestESPIDFDetector_GetArtifacts_FlashFiles(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "bootloader"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "partition_table"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tmpDir, "bootloader", "bootloader.bin"), []byte("boot"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "partition_table", "partition-table.bin"), []byte("part"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "esp32s3_hello.bin"), []byte("app-image"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "storage.bin"), []byte("fat-image"), 0644)

	// flash_args lists all four images with their real offsets.
	flashArgs := "--flash-mode dio --flash-freq 80m --flash-size 2MB\n" +
		"0x0 bootloader/bootloader.bin\n" +
		"0x8000 partition_table/partition-table.bin\n" +
		"0x50000 esp32s3_hello.bin\n" +
		"0x10000 storage.bin\n"
	os.WriteFile(filepath.Join(tmpDir, "flash_args"), []byte(flashArgs), 0644)

	d := &ESPIDFDetector{}
	artifacts, err := d.GetArtifacts(tmpDir)
	if err != nil {
		t.Fatalf("GetArtifacts: %v", err)
	}

	// All four images are present, in flash_args order, at their real offsets.
	if len(artifacts.FlashFiles) != 4 {
		t.Fatalf("expected 4 flash files, got %d: %+v", len(artifacts.FlashFiles), artifacts.FlashFiles)
	}

	want := []struct {
		base   string
		offset uint32
	}{
		{"bootloader.bin", 0x0},
		{"partition-table.bin", 0x8000},
		{"esp32s3_hello.bin", 0x50000},
		{"storage.bin", 0x10000},
	}
	for i, w := range want {
		if filepath.Base(artifacts.FlashFiles[i].Path) != w.base {
			t.Errorf("flash file[%d] path = %s, want %s", i, artifacts.FlashFiles[i].Path, w.base)
		}
		if artifacts.FlashFiles[i].Offset != w.offset {
			t.Errorf("flash file[%d] offset = 0x%x, want 0x%x", i, artifacts.FlashFiles[i].Offset, w.offset)
		}
		// Paths must be absolute so the client can read them from anywhere.
		if !filepath.IsAbs(artifacts.FlashFiles[i].Path) {
			t.Errorf("flash file[%d] path %s is not absolute", i, artifacts.FlashFiles[i].Path)
		}
	}
}
