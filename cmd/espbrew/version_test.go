package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// restoreVersion restores the package-global Version/BuildTime after a test
// mutates them so other tests are not affected.
func restoreVersion() func() {
	v, b := Version, BuildTime
	return func() { Version, BuildTime = v, b }
}

// resetVersionFlag clears the cobra `--version` flag value. Cobra reads the
// flag's value (not merely that it was changed) on every Execute, so a
// previous `--version` run would otherwise keep reporting the version.
func resetVersionFlag() {
	_ = rootCmd.Flags().Set("version", "false")
}

func TestVersionStringDefault(t *testing.T) {
	defer restoreVersion()()
	Version, BuildTime = "dev", "unknown"
	if got, want := versionString(), "dev (built unknown)"; got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionStringRelease(t *testing.T) {
	defer restoreVersion()()
	Version, BuildTime = "v0.4.0", "2026-09-16T07:59:00Z"
	if got, want := versionString(), "v0.4.0 (built 2026-09-16T07:59:00Z)"; got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionStringEmptyBuildTime(t *testing.T) {
	defer restoreVersion()()
	Version, BuildTime = "v0.4.0", ""
	got := versionString()
	if !strings.Contains(got, "(built unknown)") {
		t.Fatalf("expected 'unknown' build time fallback, got %q", got)
	}
}

func TestVersionStringEmptyVersion(t *testing.T) {
	defer restoreVersion()()
	Version, BuildTime = "", "2026-09-16T07:59:00Z"
	got := versionString()
	if !strings.HasPrefix(got, "dev (built") {
		t.Fatalf("expected 'dev' version fallback, got %q", got)
	}
}

func TestVersionStringTrimsWhitespace(t *testing.T) {
	defer restoreVersion()()
	Version, BuildTime = "  v0.4.0 ", " 2026-09-16T07:59:00Z "
	if got, want := versionString(), "v0.4.0 (built 2026-09-16T07:59:00Z)"; got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionCommandRegistered(t *testing.T) {
	var hasVersion bool
	for _, c := range rootCmd.Commands() {
		if c.Name() == "version" {
			hasVersion = true
			break
		}
	}
	if !hasVersion {
		t.Fatal("expected a 'version' subcommand to be registered")
	}
	// Cobra adds the --version flag lazily during execute(); force it so we can
	// assert its presence without running the binary.
	rootCmd.InitDefaultVersionFlag()
	if rootCmd.Flags().Lookup("version") == nil {
		t.Fatal("expected a global --version flag to be added by cobra")
	}
}

func TestVersionCommandRuns(t *testing.T) {
	defer restoreVersion()()
	Version, BuildTime = "v0.4.0", "2026-09-16T07:59:00Z"

	resetVersionFlag()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command errored: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "v0.4.0") || !strings.Contains(out, "2026-09-16T07:59:00Z") {
		t.Fatalf("version output missing information: %q", out)
	}
}

func TestVersionFlagRuns(t *testing.T) {
	defer restoreVersion()()
	Version, BuildTime = "v0.4.0", "2026-09-16T07:59:00Z"

	resetVersionFlag()
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"--version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("--version errored: %v", err)
	}
}
