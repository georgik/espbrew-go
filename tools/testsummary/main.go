// Command testsummary consumes the structured output of
//
//	go test -json ./... | testsummary
//
// and prints a concise, PROMINENT report of every failed test/package at the
// END of the run.
//
// Why this exists
// ---------------
// `go test -v ./...` runs every package together. When one package fails, go
// test prints that package's "FAIL" inline — but the packages that run after it
// still emit their "PASS"/"ok" lines, so the failure gets buried in the middle
// of a long log and is easy to miss (the run "FAIL"s, but the signal is lost).
//
// `go test -json` is the NATIVE, structured form of the exact same run. Each
// test event (run/pass/fail/output/…) is emitted as one JSON object, so we can
// aggregate every failure and emit a single clear report once the run
// completes. Tests still run through `go test` exactly as before — this only
// changes how the result is REPORTED. It invents no test framework and needs no
// third-party dependency (stdlib only).
//
// Exit code
// ---------
// Non-zero if any test or package failed, so it still fails the CI the same way
// `go test` would. When everything passes it prints a short "ALL PASS" line and
// exits 0, keeping the green log clean.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// testEvent mirrors the subset of `go test -json` fields we care about.
type testEvent struct {
	Package string `json:"package"`
	Test    string `json:"test"`
	Subtest string `json:"subtest"`
	Action  string `json:"action"`
	Output  string `json:"output"`
}

// group collects every output line seen for one package+test, plus whether that
// group ended in failure.
type group struct {
	pkg    string
	test   string
	output strings.Builder
	failed bool
}

func main() {
	dec := json.NewDecoder(os.Stdin)

	groups := map[string]*group{}
	var order []string // first-seen order, kept only for stable iteration

	// ensure returns the group for key, creating it on first sight.
	ensure := func(key string) *group {
		g := groups[key]
		if g == nil {
			g = &group{}
			groups[key] = g
			order = append(order, key)
		}
		return g
	}

	for {
		var ev testEvent
		if err := dec.Decode(&ev); err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintln(os.Stderr, "testsummary: reading test JSON:", err)
			os.Exit(2)
		}

		// Group by package + test. Subtests share the parent's key, so a
		// failing subtest's body is captured under its parent test.
		g := ensure(ev.Package + "\x00" + ev.Test)
		g.pkg = ev.Package
		g.test = ev.Test
		// Accumulate ALL output for the group. A failing test's detail
		// ("--- FAIL: …" and its body) can arrive in events before the
		// "fail" action, so we cannot gate accumulation on g.failed.
		g.output.WriteString(ev.Output)
		if ev.Action == "fail" {
			g.failed = true
		}
	}

	// Collect the failing groups in a deterministic (package, then test) order.
	var failed []*group
	for _, key := range order {
		if g := groups[key]; g.failed {
			failed = append(failed, g)
		}
	}

	if len(failed) == 0 {
		fmt.Println("TESTS PASSED — no failures.")
		os.Exit(0)
	}

	sort.SliceStable(failed, func(i, j int) bool {
		if failed[i].pkg != failed[j].pkg {
			return failed[i].pkg < failed[j].pkg
		}
		return failed[i].test < failed[j].test
	})

	var sb strings.Builder
	fmt.Fprintln(&sb, "============================================================")
	fmt.Fprintf(&sb, "TEST FAILURES: %d package(s)/test(s)\n", len(failed))
	fmt.Fprintln(&sb, "============================================================")
	for _, g := range failed {
		name := g.test
		if name == "" {
			name = g.pkg // package-level failure (e.g. a build/compile error)
		}
		fmt.Fprintf(&sb, "\n### FAIL  %s\n", name)
		body := strings.TrimRight(g.output.String(), "\n")
		if strings.TrimSpace(body) == "" {
			fmt.Fprintln(&sb, "    (no output captured)")
		} else {
			for _, line := range strings.Split(body, "\n") {
				fmt.Fprintf(&sb, "    %s\n", line)
			}
		}
	}
	fmt.Fprintln(&sb, "============================================================")
	fmt.Fprintln(&sb, "RESULT: FAIL")
	fmt.Fprintln(&sb, "============================================================")

	// Emit to stdout so the report lands at the END of the run, after every
	// package's own output.
	fmt.Print(sb.String())
	os.Exit(1)
}
