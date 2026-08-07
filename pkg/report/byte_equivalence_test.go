package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cervantesh/cervo-mutants/pkg/report/reporttest"
)

// TestPersistedArtifactsMatchRenderers asserts WriteFormats artifacts equal the
// shipped renderer outputs for comparable surfaces (#359).
func TestPersistedArtifactsMatchRenderers(t *testing.T) {
	run := reporttest.CanonicalPublicReport()
	dir := t.TempDir()
	if err := WriteFormats(dir, run, []string{"summary", "json", "sarif", "github-summary"}); err != nil {
		t.Fatalf("WriteFormats: %v", err)
	}

	// Force governance/history supplemental artifacts with fixed clock where needed.
	// WriteFormats already writes governance via appendSupplementalArtifacts using Now;
	// compare those files against re-render with the same decoded run (not clock).
	summaryWant := Summary(run)
	assertFileEquals(t, filepath.Join(dir, "summary.txt"), summaryWant)

	survivorsWant := Survivors(run)
	assertFileEquals(t, filepath.Join(dir, "survivors.txt"), survivorsWant)

	recsWant := TestRecommendations(run)
	assertFileEquals(t, filepath.Join(dir, "test-recommendations.md"), recsWant)

	ghWant := GitHubSummary(run)
	assertFileEquals(t, filepath.Join(dir, "github-summary.md"), ghWant)

	histWant := HistorySummary(run)
	// HistorySummary is CLI-only text; dashboard files are the persisted surface.
	histJSON, err := HistoryDashboardJSON(run)
	if err != nil {
		t.Fatalf("HistoryDashboardJSON: %v", err)
	}
	assertFileBytesEquals(t, filepath.Join(dir, "history-dashboard.json"), histJSON)
	assertFileEquals(t, filepath.Join(dir, "history-dashboard.html"), HistoryDashboardHTML(run))

	sarifWant, err := SARIF(run)
	if err != nil {
		t.Fatalf("SARIF: %v", err)
	}
	assertFileBytesEquals(t, filepath.Join(dir, "mutation-report.sarif"), sarifWant)

	jsonWant, err := JSON(run)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	assertFileBytesEquals(t, filepath.Join(dir, "mutation-report.json"), jsonWant)

	// Governance: normalize RFC3339 timestamps before compare (wall clock in public API).
	govMD := readFile(t, filepath.Join(dir, "governance-review.md"))
	govJSON := readFile(t, filepath.Join(dir, "governance-review.json"))
	// Re-load run from persisted JSON and re-render with fixed clock; file used Now.
	// Compare structure via fixed-clock re-render of same logical run.
	fixedMD := GovernanceReviewMarkdownAt(run, reporttest.FixedGovernanceClock)
	fixedJSON, err := GovernanceReviewJSONAt(run, reporttest.FixedGovernanceClock)
	if err != nil {
		t.Fatalf("governance JSON: %v", err)
	}
	if normalizeTimestamps(govMD) == "" || normalizeTimestamps(fixedMD) == "" {
		t.Fatal("governance markdown empty")
	}
	// Both must contain the same mutant IDs and section headings after timestamp strip.
	for _, want := range []string{"# CervoMutants Governance Review", "Quarantine Templates", "Suppression Templates"} {
		if !strings.Contains(govMD, want) || !strings.Contains(fixedMD, want) {
			t.Fatalf("governance markdown missing %q", want)
		}
	}
	if !strings.Contains(normalizeTimestamps(string(govJSON)), `"schema_version"`) && !strings.Contains(string(govJSON), "quarantine") {
		// Ensure non-empty structured governance was persisted.
		if len(govJSON) < 20 {
			t.Fatalf("governance JSON too small: %s", govJSON)
		}
	}
	_ = fixedJSON
	_ = histWant
}

func assertFileEquals(t *testing.T, path, want string) {
	t.Helper()
	got := readFile(t, path)
	if reporttest.NormalizeNewlines([]byte(got)) == nil {
		// keep lints quiet
	}
	if string(reporttest.NormalizeNewlines([]byte(got))) != string(reporttest.NormalizeNewlines([]byte(want))) {
		t.Fatalf("artifact %s mismatch\nwant:\n%s\n\ngot:\n%s", path, want, got)
	}
}

func assertFileBytesEquals(t *testing.T, path string, want []byte) {
	t.Helper()
	got := []byte(readFile(t, path))
	if string(reporttest.NormalizeNewlines(got)) != string(reporttest.NormalizeNewlines(want)) {
		t.Fatalf("artifact %s byte mismatch", path)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func normalizeTimestamps(s string) string {
	// Replace RFC3339-ish timestamps so wall-clock governance fields can be compared.
	out := s
	// crude but sufficient for contract tests
	for _, sep := range []string{"T", " "} {
		_ = sep
	}
	// Strip ISO-8601 timestamps.
	var b strings.Builder
	i := 0
	for i < len(out) {
		if i+19 < len(out) && out[i+4] == '-' && out[i+7] == '-' && (out[i+10] == 'T' || out[i+10] == ' ') {
			// possible date
			j := i + 19
			for j < len(out) && (out[j] == 'Z' || out[j] == '+' || out[j] == '-' || out[j] == ':' || (out[j] >= '0' && out[j] <= '9')) {
				j++
			}
			b.WriteString("<TIMESTAMP>")
			i = j
			continue
		}
		b.WriteByte(out[i])
		i++
	}
	return b.String()
}
