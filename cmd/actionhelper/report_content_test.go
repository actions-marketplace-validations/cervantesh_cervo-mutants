package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cervantesh/cervo-mutants/pkg/report"
	"github.com/cervantesh/cervo-mutants/pkg/report/reporttest"
)

func TestValidateMutationReportContentAcceptsCanonical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mutation-report.json")
	data, err := report.JSON(reporttest.CanonicalPublicReport())
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateMutationReportContent(path); err != nil {
		t.Fatalf("canonical should pass: %v", err)
	}
}

func TestValidateMutationReportContentRejectsDegraded(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "missing-schema", body: `{"summary":{"score":1,"actionable":{}}}`},
		{name: "missing-score", body: `{"schema_version":"1","summary":{"actionable":{}}}`},
		{name: "empty-mutants-with-generated", body: `{"schema_version":"1","summary":{"score":50,"generated_mutants":3,"actionable":{"actionable_score":50}},"mutants":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := validateMutationReportContent(path); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestValidateMutationReportContentAcceptsPreviousRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mutation-report.json")
	if err := os.WriteFile(path, []byte(reporttest.PreviousReleaseMutationReportJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateMutationReportContent(path); err != nil {
		t.Fatalf("previous-release fixture should pass: %v", err)
	}
}
