package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRustESPDetector_Name(t *testing.T) {
	detector := &RustESPDetector{}
	assert.Equal(t, "rust-esp", detector.Name())
}

func TestRustESPDetector_Detect(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{
			name: "valid Rust ESP32-S3 project",
			files: map[string]string{
				"Cargo.toml": `
[package]
name = "esp32-project"
version = "0.1.0"
edition = "2021"

[dependencies]
esp-hal = { version = "1.0", features = ["esp32s3"] }
esp-backtrace = "0.19"
esp-println = "0.17"
`,
				".cargo/config.toml": `
[target.xtensa-esp32s3-none-elf]
runner = "espflash flash"

[build]
target = "xtensa-esp32s3-none-elf"
`,
			},
			want: true,
		},
		{
			name: "Rust project without ESP dependencies",
			files: map[string]string{
				"Cargo.toml": `
[package]
name = "regular-rust"
version = "0.1.0"

[dependencies]
serde = "1.0"
`,
				".cargo/config.toml": `[build]
target = "x86_64-unknown-linux-gnu"
`,
			},
			want: false,
		},
		{
			name: "missing .cargo config",
			files: map[string]string{
				"Cargo.toml": `
[dependencies]
esp-hal = "1.0"
`,
			},
			want: false,
		},
		{
			name: "Cargo.toml with esp-idf-sys",
			files: map[string]string{
				"Cargo.toml": `
[dependencies]
esp-idf-sys = "0.33"
`,
				".cargo/config": `
[target.xtensa-esp32-none-elf]
runner = "espflash"
`,
			},
			want: true,
		},
		{
			name: "Rust ESP32-C3 project (riscv)",
			files: map[string]string{
				"Cargo.toml": `
[dependencies]
esp-hal = { version = "1.0", features = ["esp32c3"] }
`,
				".cargo/config.toml": `
[target.riscv32imc-unknown-none-elf]
runner = "espflash flash"
`,
			},
			want: true,
		},
		{
			name: ".cargo/config (not config.toml)",
			files: map[string]string{
				"Cargo.toml": `
[dependencies]
esp-hal = "1.0"
esp-backtrace = "0.19"
`,
				".cargo/config": `[target.xtensa-esp32-none-elf]
runner = "espflash"
`,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			createFiles(t, tmpDir, tt.files)

			detector := &RustESPDetector{}
			got := detector.Detect(tmpDir)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRustESPDetector_FindBuildDir(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string) error
		wantErr   bool
		checkPath func(*testing.T, string)
	}{
		{
			name: "target directory exists from config",
			setup: func(tmpDir string) error {
				if err := os.MkdirAll(filepath.Join(tmpDir, ".cargo"), 0755); err != nil {
					return err
				}
				config := `[target.xtensa-esp32s3-none-elf]
runner = "espflash"
`
				if err := os.WriteFile(filepath.Join(tmpDir, ".cargo", "config.toml"), []byte(config), 0644); err != nil {
					return err
				}
				return os.MkdirAll(filepath.Join(tmpDir, "target", "xtensa-esp32s3-none-elf", "release"), 0755)
			},
			wantErr: false,
			checkPath: func(t *testing.T, path string) {
				assert.Contains(t, path, "target")
				assert.Contains(t, path, "release")
			},
		},
		{
			name: "finds ESP32 target without config",
			setup: func(tmpDir string) error {
				return os.MkdirAll(filepath.Join(tmpDir, "target", "xtensa-esp32-none-elf", "release"), 0755)
			},
			wantErr: false,
			checkPath: func(t *testing.T, path string) {
				assert.Contains(t, path, "xtensa-esp32-none-elf")
			},
		},
		{
			name: "no target directory",
			setup: func(string) error {
				return nil
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

			detector := &RustESPDetector{}
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

func TestRustESPDetector_GetArtifacts(t *testing.T) {
	t.Run("finds ELF binary", func(t *testing.T) {
		tmpDir := t.TempDir()
		elfPath := filepath.Join(tmpDir, "esp32-project")
		elfContent := []byte{0x7F, 'E', 'L', 'F', 0x01, 0x02}
		for i := 0; i < 10000; i++ {
			elfContent = append(elfContent, 0x00)
		}
		require.NoError(t, os.WriteFile(elfPath, elfContent, 0755))

		detector := &RustESPDetector{}
		artifacts, err := detector.GetArtifacts(tmpDir)

		require.NoError(t, err)
		assert.NotEmpty(t, artifacts.App)
		assert.Contains(t, artifacts.App, "esp32-project")
	})

	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		detector := &RustESPDetector{}
		artifacts, err := detector.GetArtifacts(tmpDir)

		require.NoError(t, err)
		assert.Empty(t, artifacts.App)
	})

	t.Run("skips build artifacts", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create various build artifact files that should be skipped
		os.WriteFile(filepath.Join(tmpDir, "test.d"), []byte("test"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "test.o"), []byte("test"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "lib.a"), []byte("test"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "dep.rmeta"), []byte("test"), 0644)

		detector := &RustESPDetector{}
		artifacts, err := detector.GetArtifacts(tmpDir)

		require.NoError(t, err)
		assert.Empty(t, artifacts.App)
	})
}

func TestRustESPDetector_extractTargetTriple(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		want    string
		wantErr bool
	}{
		{
			name: "target section",
			config: `[target.xtensa-esp32s3-none-elf]
runner = "espflash"
`,
			want:    "xtensa-esp32s3-none-elf",
			wantErr: false,
		},
		{
			name: "build target",
			config: `[build]
target = "xtensa-esp32-none-elf"
`,
			want:    "xtensa-esp32-none-elf",
			wantErr: false,
		},
		{
			name: "empty config",
			config: `# Just a comment
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &RustESPDetector{}
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")
			require.NoError(t, os.WriteFile(configPath, []byte(tt.config), 0644))

			got, err := detector.extractTargetTriple(configPath)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
