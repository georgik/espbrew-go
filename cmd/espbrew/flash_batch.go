package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/georgik/espbrew-go/internal/cluster"
	"github.com/georgik/espbrew-go/internal/flash"
	"github.com/georgik/espbrew-go/internal/project"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

// batchCmd implements espbrew's batch / manifest-driven flash command. It moves
// the orchestration that a CI shell script used to perform (loop, cp into
// place, backgrounding, PID tracking, per-board files, banner printing, summary)
// into espbrew itself: one client submits every board's flash job to the cluster
// and the leader's worker pool flashes them in parallel. The workflow then
// becomes a single `espbrew flash --manifest boards.yaml` call.
//
// See wiki/espbrew-recommendation.md for the full rationale.
var batchCmd = &cobra.Command{
	Use:   "flash-batch",
	Short: "Flash + monitor all boards from a manifest (CI-friendly)",
	Long: `Flash and monitor every board listed in a manifest in a single invocation.

espbrew reads the manifest (YAML or JSON), resolves each board's images from
its build directory (honouring build/flash_args offsets), uploads and submits
one flash job per image to the cluster, waits for every job, then optionally
monitors each board. The leader's worker pool flashes the boards in parallel, so
the shell no longer has to background processes, track PIDs, or print a summary.

Provide the manifest with --manifest <file>, or derive the board list from a
directory of downloaded build artifacts with --artifacts-dir.`,
	RunE: runBatch,
}

var batchOpts struct {
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
}

func init() {
	batchCmd.Flags().StringVar(&batchOpts.manifestPath, "manifest", "", "Path to a board manifest (YAML or JSON)")
	batchCmd.Flags().StringVar(&batchOpts.artifactsDir, "artifacts-dir", "", "Directory of flash-files-* build artifacts to auto-map to build dirs")
	batchCmd.Flags().StringVar(&batchOpts.artifactsLayout, "artifacts-layout", "flash-files-<example>", "Layout mapping an artifact name to a build dir; <example> is substituted")
	batchCmd.Flags().StringVar(&batchOpts.clusterURL, "cluster", os.Getenv("ESPBREW_CLUSTER"), "Cluster URL for remote flashing")
	batchCmd.Flags().IntVar(&batchOpts.monitorDuration, "monitor-duration", 0, "Monitor duration in seconds per board after flash (0=skip)")
	batchCmd.Flags().IntVar(&batchOpts.monitorBaud, "monitor-baud", 115200, "Monitor baud rate")
	batchCmd.Flags().StringVar(&batchOpts.exitOnError, "exit-on-error", "", "Serial string that fails a board on monitor (boot crash)")
	batchCmd.Flags().StringVar(&batchOpts.exitOn, "exit-on", "", "Serial string that counts as success on monitor")
	batchCmd.Flags().DurationVar(&batchOpts.waitTimeout, "wait-timeout", 5*time.Minute, "Max time to wait for each board's flash to finish")
	batchCmd.Flags().DurationVar(&batchOpts.pollInterval, "poll-interval", 200*time.Millisecond, "Interval between job status polls")

	rootCmd.AddCommand(batchCmd)
}

// batchMonitorConfig holds the optional monitor settings for a board.
type batchMonitorConfig struct {
	Duration    int    `yaml:"duration"`
	ExitOnError string `yaml:"exit_on_error"`
	ExitOn      string `yaml:"exit_on"`
}

// batchBoardConfig is one board entry in the manifest.
type batchBoardConfig struct {
	Alias    string             `yaml:"alias"`
	BuildDir string             `yaml:"build_dir"`
	Monitor  batchMonitorConfig `yaml:"monitor"`
	Erase    bool               `yaml:"erase"`
	Optional bool               `yaml:"optional"`
}

// batchManifest is the top-level manifest document.
type batchManifest struct {
	Boards []batchBoardConfig `yaml:"boards"`
}

// boardResult records the outcome of processing one board.
type boardResult struct {
	alias    string
	images   int
	jobIDs   []string
	err      error
	optional bool
	skipped  bool
}

func runBatch(cmd *cobra.Command, args []string) error {
	manifest, err := loadManifest()
	if err != nil {
		return err
	}
	if len(manifest.Boards) == 0 {
		return fmt.Errorf("no boards in manifest")
	}

	if batchOpts.clusterURL == "" {
		return fmt.Errorf("--cluster (or ESPBREW_CLUSTER) is required for flash-batch")
	}

	client := cluster.NewClient(batchOpts.clusterURL)

	// Phase 1: flash every board. Each board is flashed in its own goroutine so
	// distinct devices flash in parallel (the leader's worker pool runs them
	// concurrently); within a board, images flash one at a time because the
	// leader reserves the device when a job is submitted.
	results := make([]boardResult, len(manifest.Boards))
	var wg sync.WaitGroup
	for i := range manifest.Boards {
		board := manifest.Boards[i]
		wg.Add(1)
		go func(i int, board batchBoardConfig) {
			defer wg.Done()
			results[i] = flashBoard(client, board)
		}(i, board)
	}
	wg.Wait()

	// Phase 2: monitor each board that flashed successfully. Monitoring is a
	// separate phase so the device reserve happens after flash releases it (the
	// leader locks a device per job).
	if batchOpts.monitorDuration > 0 {
		for i := range manifest.Boards {
			if results[i].err != nil || results[i].skipped {
				continue
			}
			if err := runBatchMonitor(results[i].alias); err != nil {
				results[i].err = err
			}
		}
	}

	reportBatch(results)

	// Exit non-zero if any non-optional board failed so the CI job fails.
	for _, r := range results {
		if r.err != nil && !r.optional {
			return fmt.Errorf("board %s failed: %w", r.alias, r.err)
		}
	}
	return nil
}

// flashBoard resolves a board's device and images, then flashes them and returns
// the board's result. Images flash one at a time: the leader reserves the
// device when a job is submitted, so the next image can only be submitted after
// the previous one completes — the same sequential behaviour the single `flash`
// command uses for multi-image flashes (flashing all images at once would make
// every submit after the first fail with "device not available").
func flashBoard(client *cluster.Client, board batchBoardConfig) boardResult {
	res := boardResult{alias: board.Alias, optional: board.Optional}

	images, err := resolveBoardImages(board.BuildDir)
	if err != nil {
		failOrSkip(&res, board, fmt.Errorf("resolve images: %w", err))
		return res
	}

	var jobIDs []string
	for _, img := range images {
		start := time.Now()
		ciFlashLine(board.Alias, img.name, "flashing…")

		uploadResp, err := client.UploadFirmware(img.path)
		if err != nil {
			failOrSkip(&res, board, fmt.Errorf("upload %s: %w", img.name, err))
			return res
		}

		// Address the board by alias. The server resolves alias -> device, so
		// the client never needs (or learns) the physical /dev path.
		flashResp, err := client.SubmitFlash(cluster.FlashSubmitRequest{
			DeviceAlias: board.Alias,
			FileID:      uploadResp.FileID,
			ClientID:    "espbrew-cli",
			Offset:      img.offset,
			Erase:       board.Erase,
		})
		if err != nil {
			failOrSkip(&res, board, fmt.Errorf("submit %s: %w", img.name, err))
			return res
		}
		jobIDs = append(jobIDs, flashResp.JobID)
		log.Debug().Str("alias", board.Alias).Str("image", img.name).Str("job_id", flashResp.JobID).Msg("Flash job submitted")

		// Wait for this image to reach a terminal state before submitting the
		// next one so the device reservation is released in time.
		job, err := waitForJob(client, flashResp.JobID, batchOpts.waitTimeout, batchOpts.pollInterval)
		if err != nil {
			failOrSkip(&res, board, err)
			return res
		}
		if job.Status == "failed" {
			failOrSkip(&res, board, fmt.Errorf("flash %s: job %s failed: %s", img.name, flashResp.JobID, job.Error))
			return res
		}
		ciFlashOK(board.Alias, img.name, time.Since(start))
	}

	res.images = len(images)
	res.jobIDs = jobIDs
	ciFlashOK(board.Alias, fmt.Sprintf("%d image(s)", len(images)), 0)
	return res
}

// failOrSkip records a board-level failure, or marks the board skipped when it
// is optional (e.g. a missing build artifact), and emits the matching CI
// annotation. Optional boards never fail the batch.
func failOrSkip(res *boardResult, board batchBoardConfig, err error) {
	if board.Optional {
		ciWarning(fmt.Sprintf("%s: %v — skipping", res.alias, err))
		res.skipped = true
		return
	}
	ciFlashFail(res.alias, "", err)
	res.err = err
}

// resolveBatchDevice resolves a board alias to a physical device path on the
// cluster, reusing the shared filterDevices helper.
func resolveBatchDevice(client *cluster.Client, alias string) (string, error) {
	devices, err := client.ListDevices()
	if err != nil {
		return "", err
	}
	filtered, err := filterDevices(devices, "", "", alias, nil)
	if err != nil {
		return "", err
	}
	for _, d := range filtered {
		if d.State == "available" {
			return d.Path, nil
		}
	}
	// Fall back to the first matched device even if not "available" (it may be
	// reserved by a previous phase); the leader will reject a bad submit.
	if len(filtered) > 0 {
		return filtered[0].Path, nil
	}
	return "", fmt.Errorf("no device matches alias %q", alias)
}

// resolveBoardImages resolves the ordered flash images for a board from its
// build directory. It first tries project detection (a project root containing
// CMakeLists.txt + sdkconfig + build/); if that yields no images it falls back
// to parsing build/flash_args directly, which is what an artifact directory
// (only build outputs, no project root) needs. Either way the real flash_args
// offsets are honoured, so no --chip is required.
func resolveBoardImages(buildDir string) ([]remoteImage, error) {
	if projType, detector := projectRegistry.Detect(buildDir); projType != project.ProjectTypeNone {
		if bd, err := detector.FindBuildDir(buildDir); err == nil {
			if art, err := detector.GetArtifacts(bd); err == nil {
				images := buildMultiImageImages(art.FlashFiles, art.ExtraFiles)
				if len(images) > 0 {
					return images, nil
				}
			}
		}
	}
	return parseFlashArgsImages(buildDir)
}

// parseFlashArgsImages parses build/flash_args directly and returns the ordered
// images with their real offsets. Used when buildDir already contains the build
// outputs (e.g. a downloaded artifact directory).
func parseFlashArgsImages(buildDir string) ([]remoteImage, error) {
	flashArgsPath, err := flash.FindFlashArgs(buildDir)
	if err != nil {
		return nil, fmt.Errorf("flash_args not found in %s: %w", buildDir, err)
	}

	data, err := os.ReadFile(flashArgsPath)
	if err != nil {
		return nil, fmt.Errorf("read flash_args: %w", err)
	}

	parsed, err := flash.ParseFlashArgs(data)
	if err != nil {
		return nil, fmt.Errorf("parse flash_args: %w", err)
	}

	var images []remoteImage
	for _, file := range parsed.Files {
		resolved := flash.ResolveBuildPath(buildDir, file.Path)
		if _, err := os.Stat(resolved); err != nil {
			return nil, fmt.Errorf("image %s not found: %w", file.Path, err)
		}
		images = append(images, remoteImage{
			name:   fmt.Sprintf("%s @ 0x%x", file.Path, file.Offset),
			path:   resolved,
			offset: int(file.Offset),
		})
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no files to flash in flash_args")
	}
	return images, nil
}

// waitForJob polls a job until it reaches a terminal state (completed/failed)
// or the timeout elapses. It is separated from runBatch so it can be tested
// against a mock HTTP server without real hardware.
func waitForJob(client *cluster.Client, jobID string, timeout, poll time.Duration) (*cluster.ClientJobStatus, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		job, err := client.GetJob(jobID)
		if err != nil {
			return nil, err
		}
		if job.Status == "completed" || job.Status == "failed" {
			return job, nil
		}
		select {
		case <-time.After(time.Until(deadline)):
			return job, fmt.Errorf("timed out waiting for job %s (last status: %s)", jobID, job.Status)
		case <-ticker.C:
		}
	}
}

// runBatchMonitor reserves the device and streams the serial console for the
// configured duration, annotating exit patterns in CI mode.
func runBatchMonitor(alias string) error {
	clientID := "espbrew-monitor-" + randomID(8)
	monitorClient := cluster.NewMonitorClient(batchOpts.clusterURL, alias, cluster.MonitorConfig{
		Baud:     batchOpts.monitorBaud,
		Duration: time.Duration(batchOpts.monitorDuration) * time.Second,
	})

	log.Info().Str("alias", alias).Msg("Reserving device for monitoring")
	if err := monitorClient.ReserveDevice(clientID, 300); err != nil {
		return fmt.Errorf("reserve device: %w", err)
	}
	defer monitorClient.ReleaseDevice(clientID)

	return streamBatchMonitor(monitorClient, alias)
}

func streamBatchMonitor(monitorClient *cluster.MonitorClient, alias string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer monitorClient.Close()

	dataCh := make(chan []byte, 256)
	errorCh := make(chan error, 1)
	if err := monitorClient.Stream(ctx, dataCh, errorCh); err != nil {
		return fmt.Errorf("connect monitor: %w", err)
	}

	stdoutWriter := &bufioWriter{w: os.Stdout}

	ciGroup(fmt.Sprintf("%s — monitor (%ds)", alias, batchOpts.monitorDuration))
	defer ciEndGroup()

	fmt.Printf("--- batch monitor on %s @ %d baud ---\r\n", alias, batchOpts.monitorBaud)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	exitCh := make(chan monitorExit, 1)
	go func() {
		select {
		case <-sigCh:
			exitCh <- monitorExit{success: true, message: "Interrupted"}
		case err := <-errorCh:
			if err != nil {
				exitCh <- monitorExit{success: false, message: err.Error()}
			}
		}
	}()

	timeoutCh := time.After(time.Duration(batchOpts.monitorDuration) * time.Second)

	for {
		select {
		case exit := <-exitCh:
			fmt.Printf("\n%s\n", exit.message)
			if exit.success {
				return nil
			}
			return fmt.Errorf("monitor failed: %s", exit.message)

		case <-timeoutCh:
			fmt.Printf("\nDuration limit reached (%d seconds)\n", batchOpts.monitorDuration)
			return nil

		case data, ok := <-dataCh:
			if !ok {
				return nil
			}
			stdoutWriter.Write(data)
			stdoutWriter.Flush()

			dataStr := string(data)
			if batchOpts.exitOnError != "" && contains(dataStr, batchOpts.exitOnError) {
				ciError(fmt.Sprintf("%s: boot crash — %s", alias, batchOpts.exitOnError))
				exitCh <- monitorExit{success: false, message: fmt.Sprintf("Error pattern matched: %s", batchOpts.exitOnError)}
			}
			if batchOpts.exitOn != "" && contains(dataStr, batchOpts.exitOn) {
				ciNotice(fmt.Sprintf("%s: boot ok — %s", alias, batchOpts.exitOn))
				exitCh <- monitorExit{success: true, message: fmt.Sprintf("Success pattern matched: %s", batchOpts.exitOn)}
			}
		}
	}
}

// reportBatch prints the per-board summary (GitHub annotations in CI mode) and
// returns the number of non-optional boards that failed.
func reportBatch(results []boardResult) int {
	w := ciOut()
	fmt.Fprintf(w, "\n===================== FLASH SUMMARY =====================\n")

	failed := 0
	skipped := 0
	success := []string{}
	failedAliases := []string{}
	skippedAliases := []string{}

	for _, r := range results {
		switch {
		case r.skipped:
			skipped++
			skippedAliases = append(skippedAliases, r.alias)
			fmt.Fprintf(w, "SKIPPED  %s\n", r.alias)
		case r.err != nil:
			failed++
			failedAliases = append(failedAliases, r.alias)
			fmt.Fprintf(w, "FAILED   %s\n", r.alias)
			if ciModeEnabled() {
				ciError(fmt.Sprintf("%s: %v", r.alias, r.err))
			}
		default:
			success = append(success, r.alias)
			fmt.Fprintf(w, "SUCCESS  %s\n", r.alias)
		}
	}

	fmt.Fprintf(w, "SUCCESS (%d): %s\n", len(success), joinOrNone(success))
	fmt.Fprintf(w, "FAILED   (%d): %s\n", failed, joinOrNone(failedAliases))
	fmt.Fprintf(w, "SKIPPED  (%d): %s\n", skipped, joinOrNone(skippedAliases))
	fmt.Fprintf(w, "=========================================================\n")

	return failed
}

// joinOrNone joins the items with a space, or returns "none" when empty.
func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, " ")
}

// loadManifest loads the manifest from --manifest or derives the board list from
// --artifacts-dir.
func loadManifest() (*batchManifest, error) {
	m := &batchManifest{}
	switch {
	case batchOpts.manifestPath != "":
		data, err := os.ReadFile(batchOpts.manifestPath)
		if err != nil {
			return nil, fmt.Errorf("read manifest: %w", err)
		}
		if err := decodeManifest(data, m); err != nil {
			return nil, fmt.Errorf("parse manifest: %w", err)
		}
	case batchOpts.artifactsDir != "":
		boards, err := boardsFromArtifacts()
		if err != nil {
			return nil, err
		}
		m.Boards = boards
	default:
		return nil, fmt.Errorf("provide --manifest <file> or --artifacts-dir <dir>")
	}
	return m, nil
}

// decodeManifest decodes a manifest that may be YAML or JSON. A leading '{'
// selects JSON; otherwise YAML is used (which is a superset for our needs).
func decodeManifest(data []byte, m *batchManifest) error {
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '{' {
		return json.Unmarshal(data, m)
	}
	return yaml.Unmarshal(data, m)
}

// boardsFromArtifacts derives one board per artifact directory in
// --artifacts-dir. The artifact name maps to the board alias via the layout
// template (e.g. "flash-files-<example>" -> "<example>"); the build dir is the
// artifact directory itself, which contains build/flash_args + images.
func boardsFromArtifacts() ([]batchBoardConfig, error) {
	entries, err := os.ReadDir(batchOpts.artifactsDir)
	if err != nil {
		return nil, fmt.Errorf("read artifacts dir: %w", err)
	}

	var boards []batchBoardConfig
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		boards = append(boards, batchBoardConfig{
			Alias:    artifactExample(e.Name()),
			BuildDir: filepath.Join(batchOpts.artifactsDir, e.Name()),
		})
	}

	if len(boards) == 0 {
		return nil, fmt.Errorf("no artifact directories found in %s", batchOpts.artifactsDir)
	}
	return boards, nil
}

// artifactExample maps an artifact directory name to a board alias using the
// layout template. "flash-files-<example>" -> the part after the prefix.
func artifactExample(name string) string {
	layout := strings.TrimSpace(batchOpts.artifactsLayout)
	if layout != "" {
		if idx := strings.Index(layout, "<example>"); idx >= 0 {
			prefix := layout[:idx]
			if strings.HasPrefix(name, prefix) {
				return name[len(prefix):]
			}
		}
	}
	return name
}
