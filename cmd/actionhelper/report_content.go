package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cervantesh/cervo-mutants/pkg/engine"
)

// validateMutationReportContent fails when a mutation-report.json exists but is
// empty/degraded: missing schema, missing summary scores, or empty mutant list
// when generated_mutants claims work. Used by CI/Action smoke (#364).
func validateMutationReportContent(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return fmt.Errorf("%s is empty", path)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	schema, _ := raw["schema_version"].(string)
	if schema != "1" {
		return fmt.Errorf("%s schema_version=%q want 1", path, schema)
	}
	summary, ok := raw["summary"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s missing summary object", path)
	}
	if _, ok := summary["score"]; !ok {
		return fmt.Errorf("%s summary.score missing", path)
	}
	if _, ok := summary["actionable"]; !ok {
		return fmt.Errorf("%s summary.actionable missing", path)
	}
	// Also require the typed model to decode so field renames fail loudly.
	var run engine.RunResult
	if err := json.Unmarshal(data, &run); err != nil {
		return fmt.Errorf("%s does not decode as engine.RunResult: %w", path, err)
	}
	if run.SchemaVersion != "1" {
		return fmt.Errorf("%s decoded schema_version=%q", path, run.SchemaVersion)
	}
	// Generated work without mutants is a degraded public output.
	if run.Summary.GeneratedMutants > 0 && len(run.Mutants) == 0 {
		return fmt.Errorf("%s claims generated_mutants=%d but mutants list is empty", path, run.Summary.GeneratedMutants)
	}
	return nil
}

func cmdValidateReportContent(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("validate-report-content", flag.ContinueOnError)
	path := fs.String("path", "", "path to mutation-report.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" {
		return fmt.Errorf("validate-report-content requires --path")
	}
	if err := validateMutationReportContent(*path); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "ok")
	return nil
}
