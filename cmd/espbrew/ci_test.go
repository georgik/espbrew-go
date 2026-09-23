package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// resetCIOpts restores ciOpts to a known zero state.
func resetCIOpts() {
	ciOpts = struct {
		forceCI    bool
		forceInter bool
		quiet      bool
		verbose    bool
		logLevel   string
	}{}
}

// withCIEnv runs fn with the provided CI env vars set, restoring them after.
func withCIEnv(t *testing.T, vars map[string]string, fn func()) {
	t.Helper()
	orig := map[string]string{} // vars that existed before, with their values
	added := []string{}         // vars that did not exist before and must be unset
	for k, v := range vars {
		if ev, ok := os.LookupEnv(k); ok {
			orig[k] = ev
		} else {
			added = append(added, k)
		}
		if v == "" {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, v)
		}
	}
	// Restore immediately after fn() (not via t.Cleanup) so sibling test blocks
	// observe a clean environment. defer also fires if fn panics.
	defer restoreEnv(orig, added)
	fn()
}

func restoreEnv(orig map[string]string, added []string) {
	for _, k := range added {
		os.Unsetenv(k)
	}
	for k, v := range orig {
		os.Setenv(k, v)
	}
}

func TestIsCIEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"no env", nil, false},
		{"github actions", map[string]string{"GITHUB_ACTIONS": "true"}, true},
		{"github run id", map[string]string{"GITHUB_RUN_ID": "1"}, true},
		{"gitlab", map[string]string{"GITLAB_CI": "true"}, true},
		{"jenkins", map[string]string{"JENKINS_URL": "http://ci"}, true},
		{"circleci", map[string]string{"CIRCLECI": "true"}, true},
		{"azure", map[string]string{"AZURE_PIPELINES": "true"}, true},
		{"travis", map[string]string{"TRAVIS": "true"}, true},
		{"buildkite", map[string]string{"BUILDKITE": "true"}, true},
		{"ci false", map[string]string{"GITHUB_ACTIONS": "false"}, false},
		{"ci zero", map[string]string{"CI": "0"}, false},
		{"ci empty", map[string]string{"CI": ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCIEnv(t, tt.env, func() {
				if got := isCIEnvironment(); got != tt.want {
					t.Errorf("isCIEnvironment() = %v, want %v", got, tt.want)
				}
			})
		})
	}
}

func TestCIModePrecedence(t *testing.T) {
	// --ci always forces CI mode.
	resetCIOpts()
	ciOpts.forceCI = true
	if !ciModeEnabled() {
		t.Error("expected CI mode with --ci set")
	}

	// --interactive overrides auto-detection even when GITHUB_ACTIONS is set.
	withCIEnv(t, map[string]string{"GITHUB_ACTIONS": "true"}, func() {
		resetCIOpts()
		ciOpts.forceInter = true
		if ciModeEnabled() {
			t.Error("expected interactive mode with --interactive overriding GITHUB_ACTIONS")
		}
	})

	// --quiet implies CI mode even without CI env.
	resetCIOpts()
	ciOpts.quiet = true
	if !ciModeEnabled() {
		t.Error("expected CI mode with --quiet set")
	}

	// Auto-detection: no flags, CI env present.
	withCIEnv(t, map[string]string{"GITHUB_ACTIONS": "true"}, func() {
		resetCIOpts()
		if !ciModeEnabled() {
			t.Error("expected CI mode via auto-detection")
		}
	})

	// Auto-detection: no flags, no CI env.
	resetCIOpts()
	if ciModeEnabled() {
		t.Error("expected interactive mode with no CI env and no flags")
	}
}

func TestCIAnnotations(t *testing.T) {
	origWriter := ciOutWriter

	// Interactive mode: plain text, no annotations.
	resetCIOpts()
	ciOpts.forceInter = true
	buf := &bytes.Buffer{}
	ciOutWriter = buf
	defer func() { ciOutWriter = origWriter }()

	ciNotice("board ok")
	ciWarning("skipping")
	ciError("crash")
	out := buf.String()
	if strings.Contains(out, "::") {
		t.Errorf("interactive mode must not emit annotations, got: %s", out)
	}
	if !strings.Contains(out, "board ok") {
		t.Error("expected plain message in interactive mode")
	}

	// CI mode: GitHub annotations.
	resetCIOpts()
	ciOpts.forceCI = true
	buf.Reset()
	ciNotice("board ok")
	ciWarning("skipping")
	ciError("crash")
	out = buf.String()
	for _, want := range []string{"::notice::board ok", "::warning::skipping", "::error::crash"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected annotation %q in output, got: %s", want, out)
		}
	}

	// Groups open/close in CI mode.
	buf.Reset()
	ciGroup("alias — monitor (5s)")
	ciEndGroup()
	out = buf.String()
	if !strings.Contains(out, "::group::alias — monitor (5s)::") ||
		!strings.Contains(out, "::endgroup::") {
		t.Errorf("expected group markers, got: %s", out)
	}

	// Groups are no-ops interactively.
	resetCIOpts()
	ciOpts.forceInter = true
	buf.Reset()
	ciGroup("title")
	ciEndGroup()
	if buf.Len() != 0 {
		t.Errorf("interactive mode must not emit group markers, got: %s", buf.String())
	}
}

func TestCICIConciseLines(t *testing.T) {
	origWriter := ciOutWriter

	resetCIOpts()
	ciOpts.forceCI = true
	buf := &bytes.Buffer{}
	ciOutWriter = buf
	defer func() { ciOutWriter = origWriter }()

	ciFlashLine("m5stack-atom-s3r", "bootloader", "flashing…")
	ciFlashOK("m5stack-atom-s3r", "3 image(s)", 0)
	ciFlashFail("esp-vocat", "", fmt.Errorf("boom"))

	out := buf.String()
	for _, want := range []string{
		"• m5stack-atom-s3r: bootloader flashing…",
		"✓ m5stack-atom-s3r: flashed 3 image(s)",
		"✗ esp-vocat: flash failed: boom",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected line %q, got: %s", want, out)
		}
	}

	// In interactive mode the concise lines are a no-op.
	resetCIOpts()
	ciOpts.forceInter = true
	buf.Reset()
	ciFlashLine("alias", "img", "flashing…")
	ciFlashOK("alias", "img", 0)
	ciFlashFail("alias", "", fmt.Errorf("boom"))
	if buf.Len() != 0 {
		t.Errorf("interactive mode must not emit concise flash lines, got: %s", buf.String())
	}
}

func TestResolveLogLevel(t *testing.T) {
	resetCIOpts()
	ciOpts.forceInter = true
	// Interactive, default log-level "" -> InfoLevel.
	if lvl := resolveLogLevel(); lvl != zerolog.InfoLevel {
		t.Errorf("expected InfoLevel, got %v", lvl)
	}
	// Interactive, explicit warn -> WarnLevel.
	ciOpts.logLevel = "warn"
	if lvl := resolveLogLevel(); lvl != zerolog.WarnLevel {
		t.Errorf("expected WarnLevel with --log-level=warn, got %v", lvl)
	}
	ciOpts.logLevel = ""

	// --verbose forces Debug interactively.
	ciOpts.verbose = true
	if lvl := resolveLogLevel(); lvl != zerolog.DebugLevel {
		t.Errorf("expected DebugLevel (1) with --verbose, got %v", lvl)
	}
	ciOpts.verbose = false

	// --quiet forces Warn.
	ciOpts.quiet = true
	if lvl := resolveLogLevel(); lvl != zerolog.WarnLevel {
		t.Errorf("expected WarnLevel (3) with --quiet, got %v", lvl)
	}
	ciOpts.quiet = false

	// CI env forces Warn by default.
	withCIEnv(t, map[string]string{"GITHUB_ACTIONS": "true"}, func() {
		resetCIOpts()
		if lvl := resolveLogLevel(); lvl != zerolog.WarnLevel {
			t.Errorf("expected WarnLevel (3) in CI, got %v", lvl)
		}
		// ...but --verbose still wins in CI.
		ciOpts.verbose = true
		if lvl := resolveLogLevel(); lvl != zerolog.DebugLevel {
			t.Errorf("expected DebugLevel (1) with --verbose in CI, got %v", lvl)
		}
		ciOpts.verbose = false
	})
}
