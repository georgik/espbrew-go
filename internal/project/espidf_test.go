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
		name     string
		files    map[string]string
		want     bool
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
				"CMakeLists.txt":       "cmake_minimum_required(VERSION 3.5)",
				"sdkconfig.defaults":   "CONFIG_IDF_TARGET=esp32",
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
				"CMakeLists.txt":  "cmake_minimum_required(VERSION 3.5)",
				"sdkconfig":       "CONFIG_IDF_TARGET=esp32",
				"main/main.c":     "void app_main() {}",
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
