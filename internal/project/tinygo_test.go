package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTinyGoDetector_Name(t *testing.T) {
	detector := &TinyGoDetector{}
	assert.Equal(t, "tinygo", detector.Name())
}

func TestTinyGoDetector_Detect(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{
			name: "valid TinyGo project with drivers dependency",
			files: map[string]string{
				"go.mod": `
module esp32-lcd-example

go 1.22.1

require tinygo.org/x/drivers v0.35.0
`,
			},
			want: true,
		},
		{
			name: "TinyGo project with tinygl-font dependency",
			files: map[string]string{
				"go.mod": `
module gopher-blink

go 1.22.1

require tinygo.org/x/tinygl-font v0.0.0
`,
			},
			want: true,
		},
		{
			name: "TinyGo project with machine import",
			files: map[string]string{
				"go.mod": `
module esp32-blink

go 1.22.1
`,
				"main.go": `
package main

import (
	"machine"
	"time"
)

func main() {
	led := machine.GPIO2
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	for {
		led.High()
		time.Sleep(time.Millisecond * 500)
		led.Low()
		time.Sleep(time.Millisecond * 500)
	}
}
`,
			},
			want: true,
		},
		{
			name: "regular Go project without TinyGo deps",
			files: map[string]string{
				"go.mod": `
module regular-go

go 1.22.1

require github.com/gorilla/mux v1.8.0
`,
			},
			want: false,
		},
		{
			name: "missing go.mod",
			files: map[string]string{
				"main.go": "package main\nfunc main() {}",
			},
			want: false,
		},
		{
			name:  "empty directory",
			files: map[string]string{},
			want:  false,
		},
		{
			name: "Go project with no TinyGo markers",
			files: map[string]string{
				"go.mod": `
module web-server

go 1.22.1

require github.com/gin-gonic/gin v1.9.0
`,
				"main.go": `
package main
import "github.com/gin-gonic/gin"
func main() {}
`,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			createFiles(t, tmpDir, tt.files)

			detector := &TinyGoDetector{}
			got := detector.Detect(tmpDir)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTinyGoDetector_FindBuildDir(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string) error
		wantErr   bool
		checkPath func(*testing.T, string)
	}{
		{
			name: "returns project directory as build dir",
			setup: func(tmpDir string) error {
				return nil
			},
			wantErr: false,
			checkPath: func(t *testing.T, path string) {
				assert.NotEmpty(t, path)
				assert.DirExists(t, path)
			},
		},
		{
			name: "project with go.mod",
			setup: func(tmpDir string) error {
				return os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
			},
			wantErr: false,
			checkPath: func(t *testing.T, path string) {
				assert.NotEmpty(t, path)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatal(err)
			}

			detector := &TinyGoDetector{}
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

func TestTinyGoDetector_GetArtifacts(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string) error
		wantEmpty bool
		checkFunc func(*testing.T, *BuildArtifacts)
	}{
		{
			name: "finds ELF file matching module name",
			setup: func(tmpDir string) error {
				if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module my-project"), 0644); err != nil {
					return err
				}
				elfContent := []byte{0x7F, 'E', 'L', 'F', 0x01, 0x02}
				for i := 0; i < 10000; i++ {
					elfContent = append(elfContent, 0x00)
				}
				return os.WriteFile(filepath.Join(tmpDir, "my-project"), elfContent, 0755)
			},
			checkFunc: func(t *testing.T, a *BuildArtifacts) {
				assert.NotEmpty(t, a.App)
				assert.Contains(t, a.App, "my-project")
			},
		},
		{
			name: "finds ELF file when no module match",
			setup: func(tmpDir string) error {
				elfContent := []byte{0x7F, 'E', 'L', 'F', 0x01, 0x02}
				for i := 0; i < 10000; i++ {
					elfContent = append(elfContent, 0x00)
				}
				return os.WriteFile(filepath.Join(tmpDir, "firmware"), elfContent, 0755)
			},
			checkFunc: func(t *testing.T, a *BuildArtifacts) {
				assert.NotEmpty(t, a.App)
				assert.Contains(t, a.App, "firmware")
			},
		},
		{
			name: "skips .go and .mod files",
			setup: func(tmpDir string) error {
				os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
				os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
				os.WriteFile(filepath.Join(tmpDir, "go.sum"), []byte(""), 0644)
				return nil
			},
			wantEmpty: true,
		},
		{
			name:      "empty directory",
			setup:     func(string) error { return nil },
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatal(err)
			}

			detector := &TinyGoDetector{}
			artifacts, err := detector.GetArtifacts(tmpDir)

			require.NoError(t, err)
			if tt.wantEmpty {
				assert.Empty(t, artifacts.App)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, artifacts)
			}
		})
	}
}

func TestTinyGoDetector_extractModuleName(t *testing.T) {
	tests := []struct {
		name    string
		goMod   string
		want    string
		wantErr bool
	}{
		{
			name:  "simple module name",
			goMod: `module esp32-blink`,
			want:  "esp32-blink",
		},
		{
			name:  "module path with multiple segments",
			goMod: `module github.com/user/my-project`,
			want:  "my-project",
		},
		{
			name: "module with go version",
			goMod: `module esp32-lcd-example

go 1.22.1`,
			want: "esp32-lcd-example",
		},
		{
			name: "module with dependencies",
			goMod: `module gopher-blink

go 1.22.1

require tinygo.org/x/drivers v0.35.0`,
			want: "gopher-blink",
		},
		{
			name:  "empty go.mod",
			goMod: ``,
			want:  "",
		},
		{
			name:    "invalid go.mod",
			goMod:   `# just a comment`,
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &TinyGoDetector{}
			tmpDir := t.TempDir()
			goModPath := filepath.Join(tmpDir, "go.mod")
			require.NoError(t, os.WriteFile(goModPath, []byte(tt.goMod), 0644))

			got := detector.extractModuleName(goModPath)

			if tt.wantErr {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestTinyGoDetector_checkFileForMachineImport(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{
			name: "simple machine import",
			code: `
package main

import (
	"machine"
	"time"
)

func main() {}
`,
			want: true,
		},
		{
			name: "machine import with alias",
			code: `
package main

import m "machine"

func main() {}
`,
			want: true,
		},
		{
			name: "dot import machine",
			code: `
package main

import . "machine"

func main() {}
`,
			want: true,
		},
		{
			name: "no machine import",
			code: `
package main

import (
	"fmt"
	"time"
)

func main() {}
`,
			want: false,
		},
		{
			name: "empty file",
			code: `package main`,
			want: false,
		},
		{
			name: "inline import",
			code: `package main
import "machine"
func main() {}`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &TinyGoDetector{}
			tmpDir := t.TempDir()
			testFile := filepath.Join(tmpDir, "test.go")
			require.NoError(t, os.WriteFile(testFile, []byte(tt.code), 0644))

			got := detector.checkFileForMachineImport(testFile)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTinyGoDetector_hasTinyGoDependencies(t *testing.T) {
	tests := []struct {
		name  string
		goMod string
		want  bool
	}{
		{
			name: "tinygo.org/x/drivers",
			goMod: `module test

require tinygo.org/x/drivers v0.35.0`,
			want: true,
		},
		{
			name: "tinygo.org/x/tinygl-font",
			goMod: `module test

require tinygo.org/x/tinygl-font v0.0.0`,
			want: true,
		},
		{
			name: "tinygo.org/x/adapters",
			goMod: `module test

require tinygo.org/x/adapters v0.1.0`,
			want: true,
		},
		{
			name: "non-TinyGo dependencies",
			goMod: `module test

require github.com/gorilla/mux v1.8.0`,
			want: false,
		},
		{
			name:  "empty go.mod",
			goMod: `module test`,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &TinyGoDetector{}
			tmpDir := t.TempDir()
			goModPath := filepath.Join(tmpDir, "go.mod")
			require.NoError(t, os.WriteFile(goModPath, []byte(tt.goMod), 0644))

			got, err := detector.hasTinyGoDependencies(goModPath)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
