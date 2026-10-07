package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// localRegistry is the detector registry owned by the project package. It is
// the single source of truth for "what project is this", shared by the flash
// command (cmd/espbrew/flash.go) and the packing logic below so detection and
// artifact collection can never drift apart. The concrete detectors are
// registered here in the same order the flash command used to register them.
var localRegistry = func() *Registry {
	r := NewRegistry()
	r.Register(&ESPIDFDetector{})
	r.Register(&RustESPDetector{})
	r.Register(&TinyGoDetector{})
	return r
}()

// PackFile is a single entry in an espbrew artifact pack. It is either a file
// to copy (Dir == false) or an empty directory to recreate (Dir == true).
//
// Dst is the destination path relative to the pack root, always using forward
// slashes. Preserving the on-disk layout is what lets espbrew's detector
// re-recognise the project after the pack is transferred and unpacked on the
// flashing machine.
type PackFile struct {
	// Src is the absolute source path to copy from. Empty when Dir == true.
	Src string
	// Dst is the destination path relative to the pack root.
	Dst string
	// Dir marks an empty directory that must be recreated (no content copied).
	Dir bool
}

// CollectPackFiles returns the minimal, espbrew-ready set of files/dirs that
// make up a transferable artifact for the project rooted at dir. It returns the
// detected ProjectType together with the ordered list of entries whose Dst
// paths preserve the on-disk layout so the detector re-recognises the project
// after unpack.
//
// This is the single source of truth for artifact packing. The `espbrew artifact
// pack`/`unpack` CLI, its `--print` shell emitter, and the GitHub workflow all
// derive their behaviour from this function so they can never drift apart — the
// shell scripts that used to hand-copy files (and forget directories such as
// .cargo) are replaced by this validated algorithm.
func CollectPackFiles(dir string) (ProjectType, []PackFile, error) {
	projType, detector := localRegistry.Detect(dir)
	switch projType {
	case ProjectTypeESPIDF:
		return projType, collectESPIDF(dir, detector), nil
	case ProjectTypeRustESP:
		return projType, collectRustESP(dir, detector), nil
	default:
		return ProjectTypeNone, nil, fmt.Errorf("unsupported or unrecognized project in %q (expected an ESP-IDF or Rust ESP project)", dir)
	}
}

// addFile records a file entry, normalising the source to an absolute path and
// the destination to a slash-separated pack-relative path, and de-duplicates by
// destination so the same image is never packed twice.
func addFile(files *[]PackFile, seen map[string]bool, src, dst string) {
	dst = filepath.ToSlash(dst)
	if seen[dst] {
		return
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		abs = src
	}
	if !fileExists(abs) {
		// A resolved-but-missing artifact (e.g. a relative fallback) is simply
		// skipped rather than causing a later copy failure.
		return
	}
	seen[dst] = true
	*files = append(*files, PackFile{Src: abs, Dst: dst})
}

// relPath returns target relative to base, falling back to target's base name
// when the relative computation fails. It is the pack-relative path used as the
// destination so the rebuilt layout mirrors the source.
func relPath(base, target string) string {
	if rel, err := filepath.Rel(base, target); err == nil && !filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Base(target)
}

// collectESPIDF gathers the markers, flash plan, and images an ESP-IDF build
// needs. The layout is preserved so espbrew's ESPIDFDetector re-recognises the
// project: root markers (CMakeLists.txt + sdkconfig), the build marker,
// build/flash_args, and every image resolved relative to build/.
func collectESPIDF(dir string, detector Detector) []PackFile {
	var files []PackFile
	seen := map[string]bool{}

	// Root project markers.
	if fileExists(filepath.Join(dir, "CMakeLists.txt")) {
		addFile(&files, seen, filepath.Join(dir, "CMakeLists.txt"), "CMakeLists.txt")
	}
	switch {
	case fileExists(filepath.Join(dir, "sdkconfig")):
		addFile(&files, seen, filepath.Join(dir, "sdkconfig"), "sdkconfig")
	case fileExists(filepath.Join(dir, "sdkconfig.defaults")):
		addFile(&files, seen, filepath.Join(dir, "sdkconfig.defaults"), "sdkconfig.defaults")
	}

	buildDir, err := detector.FindBuildDir(dir)
	if err != nil {
		return files
	}
	relBuild, relErr := filepath.Rel(dir, buildDir)
	if relErr != nil || relBuild == "" {
		relBuild = "build"
	}

	// Build marker: at least one of build.ninja / Makefile / build.ninja.txt
	// is what isBuildDir keys on to treat build/ as a build directory.
	for _, m := range []string{"build.ninja", "Makefile", "build.ninja.txt"} {
		if fileExists(filepath.Join(buildDir, m)) {
			addFile(&files, seen, filepath.Join(buildDir, m), filepath.Join(relBuild, m))
			break
		}
	}

	// flash_args is the authoritative flash plan (offsets + filenames).
	if fa := filepath.Join(buildDir, "flash_args"); fileExists(fa) {
		addFile(&files, seen, fa, filepath.Join(relBuild, "flash_args"))
	}

	// Images: the authoritative flash plan first (it includes the standard
	// three plus any extra data partitions), then the standard three as a
	// fallback when flash_args is missing or malformed.
	if artifacts, aerr := detector.GetArtifacts(buildDir); aerr == nil {
		for _, ff := range artifacts.FlashFiles {
			addFile(&files, seen, ff.Path, filepath.Join(relBuild, relPath(buildDir, ff.Path)))
		}
		for _, p := range []string{artifacts.Bootloader, artifacts.Partitions, artifacts.App} {
			addFile(&files, seen, p, filepath.Join(relBuild, relPath(buildDir, p)))
		}
	}
	return files
}

// collectRustESP gathers the markers and the release ELF for a Rust no_std ESP
// project, preserving the target/<triple>/release layout espbrew's
// RustESPDetector expects.
func collectRustESP(dir string, detector Detector) []PackFile {
	var files []PackFile
	seen := map[string]bool{}

	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		addFile(&files, seen, filepath.Join(dir, "Cargo.toml"), "Cargo.toml")
	}

	// .cargo/config.toml is checked first, then .cargo/config.
	cfg := filepath.Join(dir, ".cargo", "config.toml")
	if !fileExists(cfg) {
		cfg = filepath.Join(dir, ".cargo", "config")
	}
	if fileExists(cfg) {
		addFile(&files, seen, cfg, filepath.Join(".cargo", filepath.Base(cfg)))
	}

	// The ELF lives at target/<triple>/release/<elf>; FindBuildDir resolves the
	// triple from .cargo/config and GetArtifacts selects the ELF.
	if buildDir, err := detector.FindBuildDir(dir); err == nil {
		if artifacts, aerr := detector.GetArtifacts(buildDir); aerr == nil && artifacts.App != "" {
			addFile(&files, seen, artifacts.App, filepath.Join(relPath(dir, buildDir), relPath(buildDir, artifacts.App)))
		}
	}
	return files
}

// SortedFiles returns the pack entries ordered by destination so the output is
// deterministic (directories before the files that live under them, then files
// alphabetically). Deterministic ordering keeps --print output and tests stable.
func SortedFiles(files []PackFile) []PackFile {
	out := make([]PackFile, len(files))
	copy(out, files)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Dst == out[j].Dst {
			return false
		}
		// Directories sort before the paths that live beneath them so a
		// `mkdir -p` for the directory always precedes the file copy.
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return out[i].Dst < out[j].Dst
	})
	return out
}

// ExtractTargetTriple returns the ESP build target triple declared in a
// .cargo/config file: prefer a `[target.<triple>]` header, then fall back to
// `target = "<triple>"` under `[build]`. It is the shared implementation behind
// RustESPDetector and the artifact packer so both stay in lockstep.
func ExtractTargetTriple(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[target.") {
			start := len("[target.")
			if end := strings.Index(trimmed[start:], "]"); end > 0 {
				if triple := strings.TrimSpace(trimmed[start : start+end]); triple != "" && !strings.HasPrefix(triple, "$") {
					return triple
				}
			}
		}
		if strings.HasPrefix(trimmed, "target") && strings.Contains(trimmed, "=") {
			if parts := strings.SplitN(trimmed, "=", 2); len(parts) == 2 {
				triple := strings.Trim(strings.TrimSpace(parts[1]), `"`)
				if triple != "" && !strings.HasPrefix(triple, "$") {
					return triple
				}
			}
		}
	}
	return ""
}

// FindELF returns the path of the release ELF inside releaseDir, mirroring the
// selection rules espbrew uses when it converts a Rust project to images: skip
// build scratch files (.d/.o/.a/.rmeta/.rlib) and pick the largest remaining
// file (the executable). It is the shared implementation behind
// RustESPDetector.GetArtifacts and the artifact packer.
func FindELF(releaseDir string) (string, error) {
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return "", err
	}
	var (
		best    string
		bestLen int64
	)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".d") ||
			strings.HasSuffix(name, ".o") ||
			strings.HasSuffix(name, ".a") ||
			strings.HasSuffix(name, ".rmeta") ||
			strings.HasSuffix(name, ".rlib") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode().IsDir() || info.Size() < 10000 {
			continue
		}
		if best == "" || info.Size() > bestLen {
			best = entry.Name()
			bestLen = info.Size()
		}
	}
	if best == "" {
		return "", fmt.Errorf("no ELF found in %s", releaseDir)
	}
	return filepath.Join(releaseDir, best), nil
}
