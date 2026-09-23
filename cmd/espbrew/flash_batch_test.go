package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/georgik/espbrew-go/internal/cluster"
)

// resetBatchOpts restores batchOpts to a known zero state.
func resetBatchOpts() {
	batchOpts = struct {
		manifestPath    string
		artifactsDir    string
		artifactsLayout string
		clusterURL      string
		monitorDuration int
		monitorBaud     int
		exitOnError     string
		exitOn          string
		waitTimeout     time.Duration
		pollInterval    time.Duration
	}{}
}

func writeTempBuildDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	flashArgs := "--flash-mode dio --flash-freq 80m --flash-size 4m\n" +
		"0x0 bootloader/bootloader.bin\n" +
		"0x8000 partition_table/partition-table.bin\n" +
		"0x10000 firmware.bin\n"
	if err := os.WriteFile(filepath.Join(dir, "flash_args"), []byte(flashArgs), 0644); err != nil {
		t.Fatalf("write flash_args: %v", err)
	}

	for _, rel := range []string{
		"bootloader/bootloader.bin",
		"partition_table/partition-table.bin",
		"firmware.bin",
	} {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte("dummy"), 0644); err != nil {
			t.Fatalf("write image: %v", err)
		}
	}
	return dir
}

func TestParseFlashArgsImages(t *testing.T) {
	resetBatchOpts()
	dir := writeTempBuildDir(t)

	images, err := parseFlashArgsImages(dir)
	if err != nil {
		t.Fatalf("parseFlashArgsImages: %v", err)
	}
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d: %+v", len(images), images)
	}
	wantOffsets := map[string]int{
		"bootloader.bin":      0x0,
		"partition-table.bin": 0x8000,
		"firmware.bin":        0x10000,
	}
	for _, img := range images {
		want, ok := wantOffsets[baseName(img.path)]
		if !ok {
			t.Errorf("unexpected image path %q", img.path)
			continue
		}
		if img.offset != want {
			t.Errorf("image %q offset = 0x%x, want 0x%x", img.path, img.offset, want)
		}
	}
}

func TestResolveBoardImagesFallsBackToFlashArgs(t *testing.T) {
	resetBatchOpts()
	// A dir with only build outputs (no CMakeLists.txt) must still resolve via
	// the flash_args fallback.
	dir := writeTempBuildDir(t)
	images, err := resolveBoardImages(dir)
	if err != nil {
		t.Fatalf("resolveBoardImages: %v", err)
	}
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
}

func TestResolveBoardImagesMissingFlashArgs(t *testing.T) {
	resetBatchOpts()
	dir := t.TempDir()
	if _, err := resolveBoardImages(dir); err == nil {
		t.Error("expected error for dir without flash_args")
	}
}

func TestArtifactExample(t *testing.T) {
	resetBatchOpts()
	batchOpts.artifactsLayout = "flash-files-<example>"

	cases := map[string]string{
		"flash-files-esp32s3-m5stack-atom-s3r_hello": "esp32s3-m5stack-atom-s3r_hello",
		"flash-files-esp32-m5stack-core2_hello":      "esp32-m5stack-core2_hello",
		"unrelated-name":                             "unrelated-name",
	}
	for name, want := range cases {
		batchOpts.artifactsDir = "/tmp/artifacts"
		if got := artifactExample(name); got != want {
			t.Errorf("artifactExample(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestBoardsFromArtifacts(t *testing.T) {
	resetBatchOpts()
	batchOpts.artifactsDir = t.TempDir()
	batchOpts.artifactsLayout = "flash-files-<example>"

	// Create two artifact dirs, each with a flash_args + image.
	for _, name := range []string{
		"flash-files-boardA",
		"flash-files-boardB",
	} {
		dir := filepath.Join(batchOpts.artifactsDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "flash_args"), []byte("--flash-mode dio --flash-freq 80m --flash-size 4m\n0x0 firmware.bin\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "firmware.bin"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	boards, err := boardsFromArtifacts()
	if err != nil {
		t.Fatalf("boardsFromArtifacts: %v", err)
	}
	if len(boards) != 2 {
		t.Fatalf("expected 2 boards, got %d", len(boards))
	}
	got := map[string]string{boards[0].Alias: boards[0].BuildDir, boards[1].Alias: boards[1].BuildDir}
	if got["boardA"] == "" || got["boardB"] == "" {
		t.Errorf("expected aliases boardA and boardB, got %+v", got)
	}
}

func TestDecodeManifestYAMLAndJSON(t *testing.T) {
	resetBatchOpts()
	yamlDoc := `
boards:
  - alias: m5stack-atom-s3r
    build_dir: /examples/atom-s3r
    monitor:
      duration: 5
      exit_on_error: "abort() was called at PC"
  - alias: m5stack-core2
    build_dir: /examples/core2
    optional: true
`
	var m batchManifest
	if err := decodeManifest([]byte(yamlDoc), &m); err != nil {
		t.Fatalf("YAML decode: %v", err)
	}
	if len(m.Boards) != 2 {
		t.Fatalf("expected 2 boards, got %d", len(m.Boards))
	}
	if m.Boards[0].Alias != "m5stack-atom-s3r" || m.Boards[0].Monitor.Duration != 5 {
		t.Errorf("board[0] = %+v", m.Boards[0])
	}
	if !m.Boards[1].Optional {
		t.Errorf("board[1] should be optional, got %+v", m.Boards[1])
	}

	// JSON form.
	jsonDoc := `{"boards":[{"alias":"esp-vocat","build_dir":"/examples/vocat","erase":true}]}`
	var m2 batchManifest
	if err := decodeManifest([]byte(jsonDoc), &m2); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if len(m2.Boards) != 1 || !m2.Boards[0].Erase {
		t.Errorf("JSON decode mismatch: %+v", m2.Boards)
	}
}

func TestLoadManifestRequiresSource(t *testing.T) {
	resetBatchOpts()
	if _, err := loadManifest(); err == nil {
		t.Error("expected error when neither --manifest nor --artifacts-dir is set")
	}
}

// TestWaitForJob_Success verifies waitForJob returns the terminal status.
func TestWaitForJob_Success(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jobs/job-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if atomic.AddInt32(&calls, 1) == 1 {
			// First poll: still pending.
			fmt.Fprintf(w, `{"id":"job-1","status":"running","progress":50}`)
			return
		}
		fmt.Fprintf(w, `{"id":"job-1","status":"completed","progress":100}`)
	}))
	defer server.Close()

	client := cluster.NewClient(server.URL)
	client.SetRetryPolicy(1, time.Millisecond)

	job, err := waitForJob(client, "job-1", time.Second, time.Millisecond)
	if err != nil {
		t.Fatalf("waitForJob: %v", err)
	}
	if job.Status != "completed" {
		t.Errorf("expected completed, got %q", job.Status)
	}
}

// TestWaitForJob_Failed verifies a failed job is returned as terminal.
func TestWaitForJob_Failed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"job-1","status":"failed","error":"flash timeout"}`)
	}))
	defer server.Close()

	client := cluster.NewClient(server.URL)
	client.SetRetryPolicy(1, time.Millisecond)

	job, err := waitForJob(client, "job-1", time.Second, time.Millisecond)
	if err != nil {
		t.Fatalf("waitForJob: %v", err)
	}
	if job.Status != "failed" || job.Error != "flash timeout" {
		t.Errorf("unexpected result: %+v", job)
	}
}

// TestWaitForJob_Timeout verifies a stuck job returns a timeout error.
func TestWaitForJob_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"job-1","status":"running","progress":10}`)
	}))
	defer server.Close()

	client := cluster.NewClient(server.URL)
	client.SetRetryPolicy(1, time.Millisecond)

	_, err := waitForJob(client, "job-1", 10*time.Millisecond, 5*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestReportBatch(t *testing.T) {
	origWriter := ciOutWriter
	origOpts := ciOpts
	resetCIOpts()
	ciOpts.forceCI = true
	resetBatchOpts()

	buf := &bytes.Buffer{}
	ciOutWriter = buf
	defer func() {
		ciOutWriter = origWriter
		ciOpts = origOpts
	}()

	results := []boardResult{
		{alias: "board-ok", images: 3, jobIDs: []string{"j1"}},
		{alias: "board-fail", images: 3, jobIDs: []string{"j2"}, err: fmt.Errorf("flash failed")},
		{alias: "board-skip", optional: true, skipped: true, err: fmt.Errorf("no device")},
	}

	failed := reportBatch(results)
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}

	out := buf.String()
	for _, want := range []string{
		"FLASH SUMMARY",
		"SUCCESS (1): board-ok",
		"FAILED   (1): board-fail",
		"SKIPPED  (1): board-skip",
		"::error::board-fail: flash failed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected summary to contain %q, got:\n%s", want, out)
		}
	}
}

func baseName(p string) string {
	n := strings.LastIndex(p, "/")
	if n < 0 {
		return p
	}
	return p[n+1:]
}
