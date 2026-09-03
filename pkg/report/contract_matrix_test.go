package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cervantesh/cervo-mutants/pkg/engine"
	"github.com/cervantesh/cervo-mutants/pkg/report/reporttest"
)

// TestSchemaV1AdditiveRequiredFields guards structured public outputs against
// field removal/rename that would break schema v1 additive consumers (#360).
func TestSchemaV1AdditiveRequiredFields(t *testing.T) {
	run := reporttest.CanonicalPublicReport()

	jsonData, err := JSON(run)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	assertRequiredJSONPaths(t, "mutation-report.json", jsonData, []string{
		"schema_version",
		"summary",
		"summary.score",
		"summary.actionable",
		"summary.actionable.actionable_score",
		"summary.denominator_health",
		"mutants",
		"environment",
		"gate",
	})
	var decoded engine.RunResult
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("mutation-report must decode into engine.RunResult: %v", err)
	}
	if decoded.SchemaVersion != "1" {
		t.Fatalf("schema_version = %q, want 1", decoded.SchemaVersion)
	}

	gov, err := GovernanceReviewJSONAt(run, reporttest.FixedGovernanceClock)
	if err != nil {
		t.Fatalf("governance JSON: %v", err)
	}
	assertRequiredJSONPaths(t, "governance-review.json", gov, []string{
		"active_quarantine_count",
		"expired_quarantine_count",
		"quarantine_templates",
		"suppression_templates",
	})

	hist, err := HistoryDashboardJSON(run)
	if err != nil {
		t.Fatalf("history JSON: %v", err)
	}
	assertRequiredJSONPaths(t, "history-dashboard.json", hist, []string{
		"schema_version",
		"run_count",
		"latest",
		"runs",
	})
}

// TestZeroActionableFixtureLane covers healthy zero-actionable outputs (#362).
func TestZeroActionableFixtureLane(t *testing.T) {
	run := reporttest.ZeroActionablePublicReport()
	summary := Summary(run)
	for _, want := range []string{
		"Survived: 0",
		"Actionable survivors: 0",
		"Raw mutation score: 100.00%",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	gh := GitHubSummary(run)
	if !strings.Contains(strings.ToLower(gh), "no") && !strings.Contains(gh, "0") {
		// Healthy no-action lane must not invent a survivor queue with work.
		t.Fatalf("github summary unexpected for zero-actionable:\n%s", gh)
	}
	if survivors := Survivors(run); strings.TrimSpace(survivors) != "" {
		// Survivors listing should be empty when none survived.
		if strings.Contains(survivors, "actionability=high") {
			t.Fatalf("survivors should not list high actionable work:\n%s", survivors)
		}
	}
	recs := TestRecommendations(run)
	if strings.Contains(strings.ToLower(recs), "start with") && strings.Contains(recs, "priority: high") {
		t.Fatalf("recommendations should not push high-priority work on zero-actionable:\n%s", recs)
	}
	gov := GovernanceReviewMarkdownAt(run, reporttest.FixedGovernanceClock)
	if !strings.Contains(gov, "# CervoMutants Governance Review") {
		t.Fatalf("governance missing heading:\n%s", gov)
	}
	hist := HistorySummary(run)
	if !strings.Contains(hist, "Historical runs:") && !strings.Contains(hist, "No historical") {
		t.Fatalf("history unexpected:\n%s", hist)
	}
}

// TestPartialDenominatorWarningFixture covers warning-heavy partial runs (#363).
func TestPartialDenominatorWarningFixture(t *testing.T) {
	run := reporttest.PartialDenominatorWarningReport()
	summary := Summary(run)
	if !strings.Contains(summary, "Score denominator:") {
		t.Fatalf("summary missing denominator:\n%s", summary)
	}
	jsonData, err := JSON(run)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	summaryMap, _ := decoded["summary"].(map[string]any)
	health, _ := summaryMap["denominator_health"].(map[string]any)
	if health == nil {
		t.Fatal("denominator_health missing from structured output")
	}
	if healthy, _ := health["healthy"].(bool); healthy {
		t.Fatal("expected healthy=false for warning fixture")
	}
	warnings, _ := health["warnings"].([]any)
	if len(warnings) == 0 {
		t.Fatal("expected denominator warnings in structured output")
	}
	gh := GitHubSummary(run)
	if strings.TrimSpace(gh) == "" {
		t.Fatal("github summary empty for partial fixture")
	}
}

// TestPreviousReleaseMutationReportStillReadable proves current readers accept
// a previous supported release fixture (#361).
func TestPreviousReleaseMutationReportStillReadable(t *testing.T) {
	var run engine.RunResult
	if err := json.Unmarshal([]byte(reporttest.PreviousReleaseMutationReportJSON), &run); err != nil {
		t.Fatalf("previous-release fixture must decode: %v", err)
	}
	if run.SchemaVersion != "1" {
		t.Fatalf("schema_version = %q", run.SchemaVersion)
	}
	summary := Summary(run)
	if !strings.Contains(summary, "Survived: 1") {
		t.Fatalf("summary from previous-release fixture unexpected:\n%s", summary)
	}
	survivors := Survivors(run)
	if !strings.Contains(survivors, "pkg/old.go") && !strings.Contains(survivors, "logical") {
		t.Fatalf("survivors from previous-release fixture unexpected:\n%s", survivors)
	}
	if _, err := JSON(run); err != nil {
		t.Fatalf("re-encoding previous-release fixture: %v", err)
	}
	if _, err := SARIF(run); err != nil {
		t.Fatalf("SARIF from previous-release fixture: %v", err)
	}
	_ = GitHubSummary(run)
	_ = HistorySummary(run)
	_ = GovernanceReviewMarkdownAt(run, reporttest.FixedGovernanceClock)
}

func assertRequiredJSONPaths(t *testing.T, label string, data []byte, paths []string) {
	t.Helper()
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("%s: invalid JSON: %v", label, err)
	}
	for _, path := range paths {
		if !jsonPathExists(root, strings.Split(path, ".")) {
			t.Fatalf("%s missing required path %q", label, path)
		}
	}
}

func jsonPathExists(node any, parts []string) bool {
	if len(parts) == 0 {
		return true
	}
	obj, ok := node.(map[string]any)
	if !ok {
		return false
	}
	next, ok := obj[parts[0]]
	if !ok {
		return false
	}
	if len(parts) == 1 {
		return true
	}
	return jsonPathExists(next, parts[1:])
}
