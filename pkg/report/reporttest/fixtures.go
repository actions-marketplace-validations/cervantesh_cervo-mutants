package reporttest

import (
	"time"

	"github.com/cervantesh/cervo-mutants/pkg/engine"
)

// ZeroActionablePublicReport is a healthy completed run with no actionable
// survivors. Used by report and CLI contract tests for the no-action lane (#362).
func ZeroActionablePublicReport() engine.RunResult {
	run := CanonicalPublicReport()
	run.StoppedReason = "completed"
	run.Summary.Survived = 0
	run.Summary.Killed = 4
	run.Summary.NotCovered = 0
	run.Summary.TimedOut = 0
	run.Summary.Score = 100
	run.Summary.EffectiveScore = 100
	run.Summary.TestEfficacy = 100
	run.Summary.Actionable = engine.ActionableSummary{
		RawScore:                100,
		ActionableScore:         100,
		Survivors:               0,
		ActionableSurvivors:     0,
		TrueActionableSurvivors: 0,
	}
	run.Summary.HighRiskSurvivors = 0
	run.Summary.NewSurvivors = 0
	run.Summary.LongStandingSurvivors = 0
	run.Summary.PlatformSensitiveSurvivors = 0
	run.Summary.NonProgressTimeouts = 0
	run.Summary.DenominatorHealth = engine.DenominatorHealth{
		Generated:        4,
		Covered:          4,
		Executed:         4,
		Effective:        4,
		ScoreDenominator: 4,
		Killed:           4,
		Survived:         0,
		NotCovered:       0,
		TimedOut:         0,
		Healthy:          true,
	}
	run.Thresholds = map[string]any{
		"fail_under":            75,
		"fail_on_timeout":       true,
		"fail_on_compile_error": false,
		"failed":                false,
	}
	run.Gate = engine.GateEvaluation{
		Evaluated: true,
		Passed:    true,
		Checks: []engine.GateCheck{
			{Name: "fail_under", Status: engine.GateCheckPassed, Summary: "raw score 100.00% meets threshold 75"},
			{Name: "timed_out", Status: engine.GateCheckPassed, Summary: "no timed out mutants"},
			{Name: "compile_error", Status: engine.GateCheckDisabled, Summary: "compile-error gate disabled"},
		},
	}
	run.Baseline = engine.BaselineComparison{Enabled: false}
	// All mutants killed — no survivors to review.
	for i := range run.Mutants {
		if run.Mutants[i].Status == engine.StatusSurvived || run.Mutants[i].Status == engine.StatusTimedOut || run.Mutants[i].Status == engine.StatusNotCovered {
			run.Mutants[i].Status = engine.StatusKilled
			run.Mutants[i].Actionability = ""
			run.Mutants[i].SurvivorRank = 0
			run.Mutants[i].TestRecommendation = nil
			run.Mutants[i].SuggestedSkipReason = ""
		}
	}
	return run
}

// PartialDenominatorWarningReport models an incomplete/warning-heavy run (#363).
func PartialDenominatorWarningReport() engine.RunResult {
	run := CanonicalPublicReport()
	run.StoppedReason = "budget_exhausted"
	run.Summary.DenominatorHealth = engine.DenominatorHealth{
		Generated:        20,
		Covered:          10,
		Executed:         6,
		Effective:        4,
		ScoreDenominator: 10,
		Killed:           1,
		Survived:         2,
		NotCovered:       10,
		TimedOut:         1,
		Healthy:          false,
		Warnings: []string{
			"timed_out_exceeds_effective",
			"not_covered_dominates_denominator",
			"partial_run_budget_exhausted",
		},
	}
	run.Summary.GeneratedMutants = 20
	run.Summary.CoveredMutants = 10
	run.Summary.ExecutedMutants = 6
	run.Summary.ScoreDenominator = 10
	return run
}

// PreviousReleaseMutationReportJSON is a minimal schema-v1 mutation report as
// emitted by a previous supported release. Current readers must still load it (#361).
const PreviousReleaseMutationReportJSON = `{
  "schema_version": "1",
  "stopped_reason": "completed",
  "summary": {
    "total": 2,
    "killed": 1,
    "survived": 1,
    "not_covered": 0,
    "timed_out": 0,
    "generated_mutants": 2,
    "covered_mutants": 2,
    "executed_mutants": 2,
    "effective_mutants": 2,
    "score_denominator": 2,
    "score": 50,
    "effective_score": 50,
    "test_efficacy": 50,
    "mutation_coverage": 100,
    "actionable": {
      "raw_score": 50,
      "actionable_score": 50,
      "survivors": 1,
      "actionable_survivors": 1,
      "true_actionable_survivors": 1
    }
  },
  "environment": {
    "os": "linux",
    "arch": "amd64",
    "go_version": "go1.24.0",
    "tool_version": "v0.4.0"
  },
  "mutants": [
    {
      "mutant_id": "pkg/old.go:1:logical:prevrel",
      "status": "survived",
      "mutant": {
        "id": "pkg/old.go:1:logical:prevrel",
        "file": "pkg/old.go",
        "line": 1,
        "operator": "logical",
        "original": "&&",
        "mutated": "||"
      },
      "survivor_rank": 1,
      "actionability": "high"
    },
    {
      "mutant_id": "pkg/old.go:2:logical:prevkill",
      "status": "killed",
      "mutant": {
        "id": "pkg/old.go:2:logical:prevkill",
        "file": "pkg/old.go",
        "line": 2,
        "operator": "logical",
        "original": "||",
        "mutated": "&&"
      }
    }
  ]
}
`

// FixedGovernanceClock is the UTC instant contract tests use when rendering
// governance surfaces so wall-clock drift cannot fail byte comparisons.
var FixedGovernanceClock = time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
