package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keys returns the sorted Dst paths of a pack so tests can assert the exact
// set of files an artifact contains regardless of ordering.
func keys(files []PackFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Dst)
	}
	return out
}

// createFilesBytes writes a map of relative-path -> raw bytes, creating parent
// directories as needed.
func createFilesBytes(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
}

func TestCollectPackFiles_ESPIDF(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"CMakeLists.txt":                            "cmake_minimum_required(VERSION 3.5)",
		"sdkconfig":                                 "CONFIG_IDF_TARGET=esp32s3",
		"build/build.ninja":                         "# ninja",
		"build/flash_args":                          "0x0 bootloader.bin\n0x8000 partition-table.bin\n0x10000 firmware.bin\n",
		"build/bootloader/bootloader.bin":           "BOOT",
		"build/partition_table/partition-table.bin": "PART",
		"build/firmware.bin":                        "APP",
		"build/objects/should_be_ignored.o":         "junk",
	}
	createFiles(t, dir, files)

	projType, pack, err := CollectPackFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, ProjectTypeESPIDF, projType)

	got := keys(pack)
	for _, want := range []string{
		"CMakeLists.txt",
		"sdkconfig",
		"build/build.ninja",
		"build/flash_args",
		"build/bootloader/bootloader.bin",
		"build/partition_table/partition-table.bin",
		"build/firmware.bin",
	} {
		assert.Contains(t, got, want, "pack should include %q", want)
	}
	// Heavy build outputs must never be packed.
	assert.NotContains(t, got, "build/objects/should_be_ignored.o")
	// Every recorded entry points at a real, existing source file.
	for _, f := range pack {
		require.False(t, f.Dir, "ESP-IDF pack should contain no empty-dir entries")
		_, err := os.Stat(f.Src)
		assert.NoError(t, err, "src %q should exist", f.Src)
	}
}

func TestCollectPackFiles_ESPIDF_UsesSdkconfigDefaults(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"CMakeLists.txt":                  "cmake_minimum_required(VERSION 3.5)",
		"sdkconfig.defaults":              "CONFIG_IDF_TARGET=esp32",
		"build/build.ninja":               "# ninja",
		"build/flash_args":                "0x0 bootloader.bin\n",
		"build/bootloader/bootloader.bin": "BOOT",
	}
	createFiles(t, dir, files)

	projType, pack, err := CollectPackFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, ProjectTypeESPIDF, projType)
	assert.Contains(t, keys(pack), "sdkconfig.defaults")
	assert.NotContains(t, keys(pack), "sdkconfig")
}

func TestCollectPackFiles_RustESP(t *testing.T) {
	dir := t.TempDir()
	triple := "xtensa-esp32s3-none-elf"
	// The ELF must be larger than the 10 KB threshold FindELF uses.
	elf := make([]byte, 12000)
	for i := range elf {
		elf[i] = 'E'
	}
	rmeta := make([]byte, 12000)
	for i := range rmeta {
		rmeta[i] = 'R'
	}
	files := map[string][]byte{
		"Cargo.toml":         []byte("[package]\nname = \"demo\"\n\n[dependencies]\nesp-hal = \"0.15\"\n"),
		".cargo/config.toml": []byte("[build]\ntarget = \"xtensa-esp32s3-none-elf\"\n"),
		filepath.Join("target", triple, "release", "demo"):       elf,
		filepath.Join("target", triple, "release", "demo.rmeta"): rmeta,
	}
	createFilesBytes(t, dir, files)

	projType, pack, err := CollectPackFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, ProjectTypeRustESP, projType)

	got := keys(pack)
	assert.Contains(t, got, "Cargo.toml")
	assert.Contains(t, got, ".cargo/config.toml")
	assert.Contains(t, got, filepath.ToSlash("target/"+triple+"/release/demo"))
	// .rmeta scratch files must never be packed.
	assert.NotContains(t, got, filepath.ToSlash("target/"+triple+"/release/demo.rmeta"))
}

func TestCollectPackFiles_RustESP_TargetSection(t *testing.T) {
	dir := t.TempDir()
	triple := "riscv32imac-unknown-none-elf"
	elf := make([]byte, 12000)
	for i := range elf {
		elf[i] = 'E'
	}
	files := map[string][]byte{
		"Cargo.toml":         []byte("[package]\nname = \"demo\"\n\n[dependencies]\nesp-hal = \"0.15\"\n"),
		".cargo/config.toml": []byte("[target." + triple + "]\nrunner = \"espflash\"\n"),
		filepath.Join("target", triple, "release", "demo"): elf,
	}
	createFilesBytes(t, dir, files)

	projType, pack, err := CollectPackFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, ProjectTypeRustESP, projType)
	assert.Contains(t, keys(pack), filepath.ToSlash("target/"+triple+"/release/demo"))
}

func TestCollectPackFiles_Unknown(t *testing.T) {
	dir := t.TempDir()
	createFiles(t, dir, map[string]string{"README.md": "hello"})

	projType, pack, err := CollectPackFiles(dir)
	require.Error(t, err)
	assert.Equal(t, ProjectTypeNone, projType)
	assert.Empty(t, pack)
}

func TestCollectPackFiles_UnsupportedESPProject(t *testing.T) {
	// A dir with a Cargo.toml but no ESP deps and no .cargo config is not an
	// ESP project, so packing must refuse rather than produce an artifact.
	dir := t.TempDir()
	createFiles(t, dir, map[string]string{"Cargo.toml": "[package]\nname = \"not-esp\"\n"})

	_, _, err := CollectPackFiles(dir)
	assert.Error(t, err)
}

func TestSortedFiles_DirsBeforeChildren(t *testing.T) {
	files := []PackFile{
		{Dst: "build/bootloader/bootloader.bin", Src: "/x"},
		{Dst: "CMakeLists.txt", Src: "/y"},
		{Dst: "build", Dir: true},
		{Dst: "build/flash_args", Src: "/z"},
	}
	sorted := SortedFiles(files)
	got := keys(sorted)
	// "build" (the dir) must come before anything beneath it.
	iDir, iBoot, iFlash := -1, -1, -1
	for i, k := range got {
		switch k {
		case "build":
			iDir = i
		case "build/bootloader/bootloader.bin":
			iBoot = i
		case "build/flash_args":
			iFlash = i
		}
	}
	require.True(t, iDir >= 0 && iBoot >= 0 && iFlash >= 0)
	assert.Less(t, iDir, iBoot)
	assert.Less(t, iDir, iFlash)
}
