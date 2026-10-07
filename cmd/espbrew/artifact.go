package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/georgik/espbrew-go/internal/project"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// artifactCmd bundles the artifact pack/unpack helpers. These let a build step
// produce the minimal espbrew-ready project tree and a runner step rebuild it,
// without either step hand-copying files (and forgetting directories such as
// .cargo). The file-selection logic lives in the validated project.CollectPackFiles
// so the CLI, its --print emitter, and the GitHub workflow can never drift apart.
var artifactCmd = &cobra.Command{
	Use:   "artifact",
	Short: "Pack and unpack minimal espbrew-ready project artifacts",
	Long: `Pack and unpack the minimal espbrew-ready artifact for a Rust or ESP-IDF project.

"pack" selects exactly the files espbrew needs to recognise and flash a project
(Cargo.toml + .cargo/config.toml + target/<triple>/release/<elf> for Rust;
CMakeLists.txt + sdkconfig + build/{flash_args,bootloader,partition_table,app}
for ESP-IDF), preserving the on-disk layout so the detector re-recognises it.

"unpack" rebuilds a project directory from a previously packed artifact.

Use --print to emit POSIX shell commands that reproduce the operation using only
cp/mkdir, so a CI step can embed the generated commands without installing espbrew.`,
}

var artifactPackCmd = &cobra.Command{
	Use:   "pack",
	Short: "Pack the minimal espbrew-ready files from a project",
	RunE:  runArtifactPack,
}

var artifactUnpackCmd = &cobra.Command{
	Use:   "unpack",
	Short: "Reassemble a project directory from a packed artifact",
	RunE:  runArtifactUnpack,
}

var (
	artifactPackOpts struct {
		projectDir string
		outDir     string
		print      bool
	}
	artifactUnpackOpts struct {
		packDir string
		destDir string
		print   bool
	}
)

func init() {
	artifactCmd.AddCommand(artifactPackCmd)
	artifactCmd.AddCommand(artifactUnpackCmd)

	artifactPackCmd.Flags().StringVar(&artifactPackOpts.projectDir, "project", ".", "Project directory to pack")
	artifactPackCmd.Flags().StringVar(&artifactPackOpts.outDir, "out", "", "Output directory for the pack (default: pack-<project>)")
	artifactPackCmd.Flags().BoolVar(&artifactPackOpts.print, "print", false, "Print POSIX shell commands that reproduce the pack instead of executing them")

	artifactUnpackCmd.Flags().StringVar(&artifactUnpackOpts.packDir, "pack", "", "Packed artifact directory to unpack (required)")
	artifactUnpackCmd.Flags().StringVar(&artifactUnpackOpts.destDir, "dest", ".", "Destination directory to unpack into")
	artifactUnpackCmd.Flags().BoolVar(&artifactUnpackOpts.print, "print", false, "Print POSIX shell commands that reproduce the unpack instead of executing them")

	rootCmd.AddCommand(artifactCmd)
}

func runArtifactPack(cmd *cobra.Command, args []string) error {
	projectDir, err := filepath.Abs(artifactPackOpts.projectDir)
	if err != nil {
		return usageErrf("resolve project path: %s", err)
	}
	outDir := artifactPackOpts.outDir
	if outDir == "" {
		outDir = "pack-" + filepath.Base(projectDir)
	}
	outAbs, err := filepath.Abs(outDir)
	if err != nil {
		return usageErrf("resolve out path: %s", err)
	}

	projType, files, err := project.CollectPackFiles(projectDir)
	if err != nil {
		return err
	}

	if artifactPackOpts.print {
		emitPackShell(cmd.OutOrStdout(), projType, files, projectDir, outAbs)
		log.Info().Str("project", projectDir).Str("out", outAbs).
			Msg("Printed pack shell (re-run without --print to write the artifact)")
		return nil
	}

	if err := writePack(files, outAbs); err != nil {
		return err
	}
	log.Info().Str("project", string(projType)).Str("out", outAbs).
		Int("files", len(files)).Msg("Packed espbrew artifact")
	return nil
}

func runArtifactUnpack(cmd *cobra.Command, args []string) error {
	if artifactUnpackOpts.packDir == "" {
		return usageErrf("--pack is required")
	}
	packDir, err := filepath.Abs(artifactUnpackOpts.packDir)
	if err != nil {
		return usageErrf("resolve pack path: %s", err)
	}
	destAbs, err := filepath.Abs(artifactUnpackOpts.destDir)
	if err != nil {
		return usageErrf("resolve dest path: %s", err)
	}

	if artifactUnpackOpts.print {
		emitUnpackShell(cmd.OutOrStdout(), packDir, destAbs)
		log.Info().Str("pack", packDir).Str("dest", destAbs).
			Msg("Printed unpack shell (re-run without --print to write the tree)")
		return nil
	}

	if err := writeUnpack(packDir, destAbs); err != nil {
		return err
	}
	log.Info().Str("pack", packDir).Str("dest", destAbs).Msg("Unpacked espbrew artifact")
	return nil
}

// copyFile copies a single file, preserving its permission bits.
func copyArtifactFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// writePack recreates outDir from the pack entries, creating every directory
// first (empty dirs included) and then copying each file.
func writePack(files []project.PackFile, outDir string) error {
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, d := range sortedUniqueDirs(files) {
		if err := os.MkdirAll(filepath.Join(outDir, d), 0o755); err != nil {
			return err
		}
	}
	for _, f := range files {
		if f.Dir {
			continue
		}
		if err := copyArtifactFile(f.Src, filepath.Join(outDir, f.Dst)); err != nil {
			return fmt.Errorf("pack %q: %w", f.Dst, err)
		}
	}
	return nil
}

// writeUnpack recreates destDir from every regular file/directory under packDir,
// preserving the packed layout.
func writeUnpack(packDir, destDir string) error {
	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(packDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(packDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyArtifactFile(path, target)
	})
}

// emitPackShell writes POSIX shell that reproduces `espbrew artifact pack`.
// SRC/OUT are emitted as overridable variables so the generated script can be
// embedded in a workflow with the caller's own paths.
func emitPackShell(w io.Writer, projType project.ProjectType, files []project.PackFile, src, out string) {
	files = project.SortedFiles(files)
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	fmt.Fprintln(w, "#!/bin/sh")
	fmt.Fprintf(w, "# Reproduce `espbrew artifact pack --project %s --out %s` with POSIX shell only.\n", shellQuote(src), shellQuote(out))
	fmt.Fprintf(w, "# Generated by `espbrew artifact pack --print` (%s). Project type: %s.\n", now, projType)
	fmt.Fprintf(w, "# Override paths by editing the assignments or setting $ARTIFACT_SRC / $ARTIFACT_OUT.\n")
	fmt.Fprintln(w, "set -eu")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "SRC=%s\n", shellQuote(src))
	fmt.Fprintf(w, "OUT=%s\n", shellQuote(out))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "rm -rf \"$OUT\"")
	fmt.Fprintln(w, "mkdir -p \"$OUT\"")
	fmt.Fprintln(w)

	dirs := sortedUniqueDirs(files)
	if len(dirs) > 0 {
		fmt.Fprintln(w, "# Directories")
		for _, d := range dirs {
			fmt.Fprintf(w, "mkdir -p \"$OUT/%s\"\n", d)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "# Files")
	for _, f := range files {
		if f.Dir {
			continue
		}
		fmt.Fprintf(w, "cp %s \"$OUT/%s\"\n", shellQuote(f.Src), f.Dst)
	}
}

// emitUnpackShell writes POSIX shell that reproduces `espbrew artifact unpack`.
func emitUnpackShell(w io.Writer, packDir, destDir string) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	var dirs, files []string
	err := filepath.WalkDir(packDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(packDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." {
				dirs = append(dirs, rel)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		// Fall back to a plain recursive copy if the tree cannot be scanned.
		fmt.Fprintln(w, "#!/bin/sh")
		fmt.Fprintf(w, "# Reproduce `espbrew artifact unpack --pack %s --dest %s` (recursive fallback).\n", shellQuote(packDir), shellQuote(destDir))
		fmt.Fprintf(w, "# Generated by `espbrew artifact unpack --print` (%s).\n", now)
		fmt.Fprintln(w, "set -eu")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "PACK=%s\n", shellQuote(packDir))
		fmt.Fprintf(w, "DEST=%s\n", shellQuote(destDir))
		fmt.Fprintln(w)
		fmt.Fprintln(w, "rm -rf \"$DEST\"")
		fmt.Fprintf(w, "cp -r \"$PACK/.\" \"$DEST/\"\n")
		return
	}

	sort.Strings(dirs)
	sort.Strings(files)

	fmt.Fprintln(w, "#!/bin/sh")
	fmt.Fprintf(w, "# Reproduce `espbrew artifact unpack --pack %s --dest %s` with POSIX shell only.\n", shellQuote(packDir), shellQuote(destDir))
	fmt.Fprintf(w, "# Generated by `espbrew artifact unpack --print` (%s).\n", now)
	fmt.Fprintln(w, "set -eu")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "PACK=%s\n", shellQuote(packDir))
	fmt.Fprintf(w, "DEST=%s\n", shellQuote(destDir))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "rm -rf \"$DEST\"")
	fmt.Fprintln(w, "mkdir -p \"$DEST\"")
	fmt.Fprintln(w)

	if len(dirs) > 0 {
		fmt.Fprintln(w, "# Directories")
		for _, d := range dirs {
			fmt.Fprintf(w, "mkdir -p \"$DEST/%s\"\n", d)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "# Files")
	for _, f := range files {
		fmt.Fprintf(w, "cp \"$PACK/%s\" \"$DEST/%s\"\n", f, f)
	}
}

// sortedUniqueDirs returns the sorted set of all directory paths referenced by
// the pack entries (empty-dir entries plus the parent of every file).
func sortedUniqueDirs(files []project.PackFile) []string {
	set := map[string]bool{}
	for _, f := range files {
		if f.Dir {
			set[f.Dst] = true
		}
		if d := filepath.Dir(f.Dst); d != "." {
			set[d] = true
		}
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// shellQuote returns s wrapped in single quotes, escaping embedded single
// quotes per POSIX rules — safe for any path.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
