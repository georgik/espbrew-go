package snap

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ZipDirectory walks dir recursively and writes every regular file into a zip
// archive at archivePath. Each entry is stored with a path relative to dir, so
// the archive mirrors the directory structure below dir. It returns the number
// of files added.
//
// This gives callers a portable, dependency-free way to package artifacts
// (e.g. shipping snap output) without shelling out to an external `zip`
// binary that may be missing from a minimal CI image.
func ZipDirectory(archivePath, dir string) (int, error) {
	// Ensure the parent directory of the archive exists so the Create below
	// succeeds even when the caller passes a nested path.
	if parent := filepath.Dir(archivePath); parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return 0, err
		}
	}

	out, err := os.Create(archivePath)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	zw := zip.NewWriter(out)

	var count int
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			// Skip symlinks, sockets, devices, etc.
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		wr, err := zw.Create(rel)
		if err != nil {
			return err
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		if _, err := io.Copy(wr, in); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		return count, err
	}

	if err := zw.Close(); err != nil {
		return count, err
	}
	return count, nil
}
