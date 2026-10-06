package snap

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestZipDirectory(t *testing.T) {
	// Build a source directory with a flat file, a JSON file, and a nested file.
	src := t.TempDir()
	files := map[string]string{
		"snap-abc.jpg":        "image-bytes",
		"snap-abc.json":       `{"snap_id":"abc"}`,
		"nested/snap-abc.log": "log line\n",
	}
	for name, content := range files {
		p := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	archivePath := filepath.Join(t.TempDir(), "snapshot.zip")
	n, err := ZipDirectory(archivePath, src)
	if err != nil {
		t.Fatalf("ZipDirectory failed: %v", err)
	}
	if n != len(files) {
		t.Errorf("expected %d files zipped, got %d", len(files), n)
	}

	// Open the archive and verify every file round-trips with the right content.
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("failed to open archive: %v", err)
	}
	defer r.Close()

	if len(r.File) != len(files) {
		t.Errorf("expected %d entries in archive, got %d", len(files), len(r.File))
	}

	got := make(map[string]string, len(r.File))
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		got[f.Name] = string(data)
	}

	for name, content := range files {
		if _, ok := got[name]; !ok {
			t.Errorf("expected entry %q in archive; got: %v", name, keys(got))
		}
		if got[name] != content {
			t.Errorf("entry %q mismatch: got %q, want %q", name, got[name], content)
		}
	}
}

func TestZipDirectoryCreatesParentDir(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "snap-x.jpg"), []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	// archivePath lives in a not-yet-existing nested directory.
	archivePath := filepath.Join(t.TempDir(), "out", "nested", "snapshot.zip")
	if _, err := ZipDirectory(archivePath, src); err != nil {
		t.Fatalf("ZipDirectory failed to create parent dir: %v", err)
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Errorf("archive was not created: %v", err)
	}
}

func TestZipDirectoryEmptyDir(t *testing.T) {
	src := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "empty.zip")
	n, err := ZipDirectory(archivePath, src)
	if err != nil {
		t.Fatalf("ZipDirectory on empty dir failed: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 files, got %d", n)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
