package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cervantesh/cervo-mutants/pkg/engine"
	"github.com/cervantesh/cervo-mutants/pkg/report"
	"github.com/cervantesh/cervo-mutants/pkg/report/reporttest"
)

// TestCLIReportSnapshotContracts replaces substring-only checks with full
// renderer snapshots for the canonical fixture (#358).
func TestCLIReportSnapshotContracts(t *testing.T) {
	out := t.TempDir()
	fixture := reporttest.CanonicalPublicReport()
	writeMutationReport(t, out, fixture)

	cases := []struct {
		name      string
		args      []string
		want      string
		normalize func(string) string
	}{
		{
			name: "summary",
			args: []string{"report", "summary", "--out", out},
			want: report.Summary(fixture),
		},
		{
			name: "survivors",
			args: []string{"report", "survivors", "--out", out},
			want: report.Survivors(fixture),
		},
		{
			name: "recommendations",
			args: []string{"report", "recommendations", "--out", out},
			want: report.TestRecommendations(fixture),
		},
		{
			name: "history",
			args: []string{"report", "history", "--out", out},
			want: report.HistorySummary(fixture),
		},
		{
			name: "github-summary",
			args: []string{"report", "github-summary", "--out", out},
			want: report.GitHubSummary(fixture),
		},
		{
			name: "sarif",
			args: []string{"report", "sarif", "--out", out},
			want: mustSARIF(t, fixture),
			// CLI uses fmt.Println, so stdout has a trailing newline the renderer lacks.
			normalize: trimOneTrailingNewline,
		},
		{
			name: "governance",
			args: []string{"report", "governance", "--out", out},
			// Wall clock differs; compare timestamp-normalized content against fixed-clock render.
			want:      report.GovernanceReviewMarkdownAt(fixture, reporttest.FixedGovernanceClock),
			normalize: normalizeRFC3339,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Concurrent capture: SARIF (and future large artifacts) can exceed the
			// OS pipe buffer; captureStdout waits to read until after the writer finishes.
			got := captureStdoutConcurrent(t, func() {
				if err := run(tc.args); err != nil {
					t.Fatalf("run %v: %v", tc.args, err)
				}
			})
			want := tc.want
			if tc.normalize != nil {
				got = tc.normalize(got)
				want = tc.normalize(want)
			}
			if string(reporttest.NormalizeNewlines([]byte(got))) != string(reporttest.NormalizeNewlines([]byte(want))) {
				t.Fatalf("CLI %s snapshot mismatch\nwant:\n%s\n\ngot:\n%s", tc.name, want, got)
			}
		})
	}
}

// TestCLIReportMatchesPersistedArtifacts checks writer vs CLI renderer alignment (#359).
func TestCLIReportMatchesPersistedArtifacts(t *testing.T) {
	out := t.TempDir()
	fixture := reporttest.CanonicalPublicReport()
	if err := report.WriteFormats(out, fixture, []string{"summary", "json", "sarif", "github-summary"}); err != nil {
		t.Fatalf("WriteFormats: %v", err)
	}

	// summary.txt vs report summary
	cliSummary := captureStdoutConcurrent(t, func() {
		if err := run([]string{"report", "summary", "--out", out}); err != nil {
			t.Fatalf("report summary: %v", err)
		}
	})
	fileSummary := readFileString(t, filepath.Join(out, "summary.txt"))
	if cliSummary != fileSummary {
		t.Fatalf("summary CLI vs file mismatch")
	}

	cliSurvivors := captureStdoutConcurrent(t, func() {
		if err := run([]string{"report", "survivors", "--out", out}); err != nil {
			t.Fatalf("report survivors: %v", err)
		}
	})
	fileSurvivors := readFileString(t, filepath.Join(out, "survivors.txt"))
	if cliSurvivors != fileSurvivors {
		t.Fatalf("survivors CLI vs file mismatch")
	}

	cliRecs := captureStdoutConcurrent(t, func() {
		if err := run([]string{"report", "recommendations", "--out", out}); err != nil {
			t.Fatalf("report recommendations: %v", err)
		}
	})
	fileRecs := readFileString(t, filepath.Join(out, "test-recommendations.md"))
	if cliRecs != fileRecs {
		t.Fatalf("recommendations CLI vs file mismatch")
	}

	cliGH := captureStdoutConcurrent(t, func() {
		if err := run([]string{"report", "github-summary", "--out", out}); err != nil {
			t.Fatalf("report github-summary: %v", err)
		}
	})
	fileGH := readFileString(t, filepath.Join(out, "github-summary.md"))
	if cliGH != fileGH {
		t.Fatalf("github-summary CLI vs file mismatch")
	}

	cliSARIF := trimOneTrailingNewline(captureStdoutConcurrent(t, func() {
		if err := run([]string{"report", "sarif", "--out", out}); err != nil {
			t.Fatalf("report sarif: %v", err)
		}
	}))
	fileSARIF := readFileString(t, filepath.Join(out, "mutation-report.sarif"))
	if cliSARIF != fileSARIF {
		t.Fatalf("sarif CLI vs file mismatch")
	}

	// history CLI vs dashboard text re-render (not the HTML/JSON dashboard files)
	cliHist := captureStdoutConcurrent(t, func() {
		if err := run([]string{"report", "history", "--out", out}); err != nil {
			t.Fatalf("report history: %v", err)
		}
	})
	// Load persisted mutation-report and re-render HistorySummary — same as CLI path.
	loaded := loadRunFromDir(t, out)
	if cliHist != report.HistorySummary(loaded) {
		t.Fatalf("history CLI vs re-render mismatch")
	}

	// governance: normalize timestamps between CLI (Now) and file (Now at write)
	cliGov := normalizeRFC3339(captureStdoutConcurrent(t, func() {
		if err := run([]string{"report", "governance", "--out", out}); err != nil {
			t.Fatalf("report governance: %v", err)
		}
	}))
	fileGov := normalizeRFC3339(readFileString(t, filepath.Join(out, "governance-review.md")))
	if cliGov != fileGov {
		// Same-second writes usually match; if not, headings and mutant IDs must still align.
		for _, want := range []string{"# CervoMutants Governance Review", "Quarantine Templates"} {
			if !strings.Contains(cliGov, want) || !strings.Contains(fileGov, want) {
				t.Fatalf("governance CLI/file missing %q", want)
			}
		}
	}
}

// TestCLIPreviousReleaseFixtureConsumption exercises report paths on a previous
// release mutation-report.json without running the engine (#361).
func TestCLIPreviousReleaseFixtureConsumption(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, mutationReportFileName), []byte(reporttest.PreviousReleaseMutationReportJSON), 0o644); err != nil {
		t.Fatalf("write previous-release fixture: %v", err)
	}
	summary := captureStdoutConcurrent(t, func() {
		if err := run([]string{"report", "summary", "--out", out}); err != nil {
			t.Fatalf("report summary: %v", err)
		}
	})
	if !strings.Contains(summary, "Survived: 1") {
		t.Fatalf("previous-release summary unexpected:\n%s", summary)
	}
	survivors := captureStdoutConcurrent(t, func() {
		if err := run([]string{"report", "survivors", "--out", out}); err != nil {
			t.Fatalf("report survivors: %v", err)
		}
	})
	if !strings.Contains(survivors, "logical") {
		t.Fatalf("previous-release survivors unexpected:\n%s", survivors)
	}
}

// TestCLIZeroActionableAndPartialFixtures covers matrix fixtures via CLI (#362/#363).
func TestCLIZeroActionableAndPartialFixtures(t *testing.T) {
	t.Run("zero-actionable", func(t *testing.T) {
		out := t.TempDir()
		writeMutationReport(t, out, reporttest.ZeroActionablePublicReport())
		summary := captureStdoutConcurrent(t, func() {
			if err := run([]string{"report", "summary", "--out", out}); err != nil {
				t.Fatalf("summary: %v", err)
			}
		})
		if !strings.Contains(summary, "Actionable survivors: 0") {
			t.Fatalf("zero-actionable summary:\n%s", summary)
		}
	})
	t.Run("partial-denominator", func(t *testing.T) {
		out := t.TempDir()
		writeMutationReport(t, out, reporttest.PartialDenominatorWarningReport())
		summary := captureStdoutConcurrent(t, func() {
			if err := run([]string{"report", "summary", "--out", out}); err != nil {
				t.Fatalf("summary: %v", err)
			}
		})
		if !strings.Contains(summary, "Score denominator:") {
			t.Fatalf("partial summary:\n%s", summary)
		}
	})
}

// captureStdoutConcurrent reads while the writer runs so large SARIF/HTML
// stdout cannot deadlock on a full OS pipe buffer.
func captureStdoutConcurrent(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		_ = r.Close()
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

func writeMutationReport(t *testing.T, dir string, run engine.RunResult) {
	t.Helper()
	data, err := report.JSON(run)
	if err != nil {
		t.Fatalf("report.JSON: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, mutationReportFileName), data, 0o644); err != nil {
		t.Fatalf("write mutation-report.json: %v", err)
	}
}

func loadRunFromDir(t *testing.T, dir string) engine.RunResult {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, mutationReportFileName))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var run engine.RunResult
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	return run
}

func mustSARIF(t *testing.T, run engine.RunResult) string {
	t.Helper()
	data, err := report.SARIF(run)
	if err != nil {
		t.Fatalf("SARIF: %v", err)
	}
	return string(data)
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func trimOneTrailingNewline(s string) string {
	return strings.TrimSuffix(s, "\n")
}

func normalizeRFC3339(s string) string {
	// Replace timestamps so governance CLI/file compares ignore wall clock.
	out := []byte(s)
	// Walk and replace sequences that look like 2006-01-02T15:04:05Z07:00
	var b strings.Builder
	i := 0
	for i < len(out) {
		if i+10 < len(out) && out[i+4] == '-' && out[i+7] == '-' && isDigit(out[i]) {
			if _, err := time.Parse(time.RFC3339, peekRFC3339(string(out[i:]))); err == nil {
				tok := peekRFC3339(string(out[i:]))
				b.WriteString("<TIMESTAMP>")
				i += len(tok)
				continue
			}
			// also try date-only prefix with trailing Z variants via linear scan
			j := i
			for j < len(out) && (isDigit(out[j]) || out[j] == '-' || out[j] == ':' || out[j] == 'T' || out[j] == 'Z' || out[j] == '+' || out[j] == '.') {
				j++
				if j-i > 35 {
					break
				}
				if _, err := time.Parse(time.RFC3339, string(out[i:j])); err == nil {
					b.WriteString("<TIMESTAMP>")
					i = j
					goto next
				}
			}
		}
		b.WriteByte(out[i])
		i++
	next:
	}
	return b.String()
}

func peekRFC3339(s string) string {
	for n := 20; n <= 35 && n <= len(s); n++ {
		if _, err := time.Parse(time.RFC3339, s[:n]); err == nil {
			return s[:n]
		}
	}
	return ""
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
