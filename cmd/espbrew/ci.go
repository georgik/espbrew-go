package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// ciMu serializes writes to the CI output stream. Boards flash in parallel
// goroutines (see flash-batch), and without it their concurrent Fprintf calls
// could interleave mid-line and corrupt the summary/annotations.
var ciMu sync.Mutex

// ciEnvKeys is the set of environment variables that indicate a CI environment.
// The check mirrors the convention used by common CLI tools (gh, terraform, act,
// trivy): a variable counts only when present and set to a truthy value (not
// "false" or "0"). This is the basis for espbrew's automatic CI detection so it
// behaves like every other GitHub tool when running in a pipeline.
var ciEnvKeys = []string{
	"GITHUB_ACTIONS", "CI", "GITLAB_CI", "GITHUB_RUN_ID",
	"JENKINS_URL", "DRONE", "BITBUCKET_BUILD_NUMBER",
	"CIRCLECI", "AZURE_PIPELINES", "TEAMCITY_RUN", "BUILDKITE", "TRAVIS",
}

// isCIEnvironment reports whether espbrew is running inside a CI environment,
// based solely on the process environment. It is intentionally conservative:
// an unset or explicitly falsified variable does not count.
func isCIEnvironment() bool {
	for _, k := range ciEnvKeys {
		if v, ok := os.LookupEnv(k); ok && v != "" && v != "false" && v != "0" {
			return true
		}
	}
	return false
}

// ciOpts holds the resolved global output-control flags. They are bound to the
// persistent flags registered on rootCmd in main.go.
var ciOpts struct {
	forceCI    bool   // --ci: force CI-style output even if not auto-detected
	forceInter bool   // --interactive (a.k.a. --no-ci): force interactive output
	quiet      bool   // -q: reduce output (implies CI-style logging)
	verbose    bool   // -v: force verbose (more Debug lines)
	logLevel   string // base log level override (promoted from cluster to root)
}

// ciModeEnabled reports whether espbrew should behave in CI mode: concise,
// GitHub-native output with no per-percent progress bar. Precedence mirrors
// how tools like terraform/gh/trivy resolve it: an explicit flag always wins,
// then auto-detection. --quiet implies CI style even when not auto-detected.
func ciModeEnabled() bool {
	switch {
	case ciOpts.forceCI:
		return true
	case ciOpts.forceInter:
		return false
	case ciOpts.quiet:
		return true
	default:
		return isCIEnvironment()
	}
}

// resolveLogLevel computes the global zerolog level from the resolved flags.
// In CI mode the default is `warn` (the client does not need the
// "Auto-selected device"/"Firmware uploaded" chatter in a run log) unless the
// user asks for more with -v. An explicit --log-level overrides in interactive
// mode; in CI mode -v still wins so logs stay quiet by default.
func resolveLogLevel() zerolog.Level {
	if ciOpts.verbose {
		return zerolog.DebugLevel
	}
	if ciOpts.quiet {
		return zerolog.WarnLevel
	}
	if ciModeEnabled() {
		return zerolog.WarnLevel
	}
	// An explicit --log-level wins interactively. An empty value (flag default
	// not applied, e.g. in tests) falls back to InfoLevel.
	if lvlStr := strings.TrimSpace(ciOpts.logLevel); lvlStr != "" {
		if lvl, err := zerolog.ParseLevel(lvlStr); err == nil {
			return lvl
		}
	}
	return zerolog.InfoLevel
}

// ciOutWriter is the destination for CI annotations/groups. It is a package var
// (defaulting to os.Stdout) so tests can capture output. Tests reset it.
var ciOutWriter io.Writer = os.Stdout

// ciOut returns the active CI output writer.
func ciOut() io.Writer {
	if ciOutWriter != nil {
		return ciOutWriter
	}
	return os.Stdout
}

// ciAnnotate emits a GitHub Actions annotation in CI mode, or plain text in
// interactive mode. level is the annotation type (notice, warning, error).
func ciAnnotate(w io.Writer, level, msg string) {
	ciMu.Lock()
	defer ciMu.Unlock()
	if ciModeEnabled() {
		fmt.Fprintf(w, "::%s::%s\n", level, msg)
		return
	}
	fmt.Fprintln(w, msg)
}

// ciNotice emits a notice annotation (success / informational).
func ciNotice(msg string) { ciAnnotate(ciOut(), "notice", msg) }

// ciWarning emits a warning annotation (e.g. a skipped board).
func ciWarning(msg string) { ciAnnotate(ciOut(), "warning", msg) }

// ciError emits an error annotation (a failed board).
func ciError(msg string) { ciAnnotate(ciOut(), "error", msg) }

// ciGroup opens a collapsible GitHub Actions group. No-op in interactive mode.
func ciGroup(title string) {
	ciMu.Lock()
	defer ciMu.Unlock()
	if ciModeEnabled() {
		fmt.Fprintf(ciOut(), "::group::%s::\n", title)
	}
}

// ciEndGroup closes the most recent collapsible group. No-op interactively.
func ciEndGroup() {
	ciMu.Lock()
	defer ciMu.Unlock()
	if ciModeEnabled() {
		fmt.Fprintln(ciOut(), "::endgroup::")
	}
}

// ciFlashLine prints a concise one-line status for a flash step in CI mode.
// In interactive mode this is intentionally a no-op: the animated bar carries
// the progress there.
func ciFlashLine(alias, image, status string) {
	ciMu.Lock()
	defer ciMu.Unlock()
	if !ciModeEnabled() {
		return
	}
	fmt.Fprintf(ciOut(), "• %s: %s %s\n", alias, image, status)
}

// ciFlashOK prints a success line for a completed image in CI mode.
func ciFlashOK(alias, image string, d time.Duration) {
	ciMu.Lock()
	defer ciMu.Unlock()
	if !ciModeEnabled() {
		return
	}
	fmt.Fprintf(ciOut(), "✓ %s: flashed %s in %s\n", alias, image, d.Round(time.Millisecond))
}

// ciFlashFail prints a failure line for a failed image in CI mode.
func ciFlashFail(alias, image string, err error) {
	ciMu.Lock()
	defer ciMu.Unlock()
	if !ciModeEnabled() {
		return
	}
	fmt.Fprintf(ciOut(), "✗ %s: flash failed: %s\n", alias, err)
}
