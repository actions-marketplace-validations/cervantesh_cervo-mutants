package mutator

import (
	"testing"
)

func TestImportPathsAreNeverMutated(t *testing.T) {
	src := `package sample

import (
	"fmt"
	"os"
	y "gopkg.in/yaml.v3"
)

import "strings"

func Use() {
	fmt.Println(os.Getenv("HOME"))
	_ = y.Marshal
	_ = strings.TrimSpace
}
`
	importPaths := []string{
		`"fmt"`,
		`"os"`,
		`"gopkg.in/yaml.v3"`,
		`"strings"`,
	}
	for _, profile := range []string{ProfileDefault, ProfileAggressive} {
		mutants, err := Generate("sample", "sample.go", []byte(src), profile)
		if err != nil {
			t.Fatalf("Generate(%s): %v", profile, err)
		}
		for _, m := range mutants {
			for _, path := range importPaths {
				if m.Original == path {
					t.Fatalf("profile %s: import path %s must not be mutated; got operator %s mutant %+v",
						profile, path, m.Operator, m)
				}
			}
		}
	}
}

func TestStringLiteralsOutsideImportsAreStillMutated(t *testing.T) {
	src := `package sample

import "fmt"

const greeting = "hello"

func Message() string {
	s := "world"
	return s + fmt.Sprint()
}
`
	for _, profile := range []string{ProfileDefault, ProfileAggressive} {
		mutants, err := Generate("sample", "sample.go", []byte(src), profile)
		if err != nil {
			t.Fatalf("Generate(%s): %v", profile, err)
		}

		for _, original := range []string{`"hello"`, `"world"`} {
			empty := tagMutantsForOriginal(mutants, opStringEmptyLiterals, original)
			if len(empty) != 1 {
				t.Fatalf("profile %s: want 1 string-empty-literals mutant for %s, got %d in %+v",
					profile, original, len(empty), mutants)
			}
			if profile == ProfileAggressive {
				lit := tagMutantsForOriginal(mutants, opLiterals, original)
				if len(lit) != 1 {
					t.Fatalf("aggressive: want 1 literals mutant for %s, got %d", original, len(lit))
				}
			}
		}

		// Import path must still be skipped in this mixed file.
		for _, m := range mutants {
			if m.Original == `"fmt"` {
				t.Fatalf("profile %s: import path \"fmt\" must not be mutated: %+v", profile, m)
			}
		}
	}
}

func TestImportPathStringMatchingImportElsewhereIsStillMutated(t *testing.T) {
	src := `package sample

import "fmt"

func Message() string {
	return "fmt"
}
`
	mutants, err := Generate("sample", "sample.go", []byte(src), ProfileDefault)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Body literal matching an import path text must still be mutated.
	body := tagMutantsForOriginal(mutants, opStringEmptyLiterals, `"fmt"`)
	if len(body) != 1 {
		t.Fatalf("want 1 string-empty-literals mutant for body \"fmt\", got %d in %+v", len(body), mutants)
	}

	// And no mutant should come from the import itself — only the body one.
	// Count all mutants whose Original is `"fmt"` across any operator.
	var fmtOriginals []Mutant
	for _, m := range mutants {
		if m.Original == `"fmt"` {
			fmtOriginals = append(fmtOriginals, m)
		}
	}
	// Default profile: only string-empty-literals for the body literal.
	if len(fmtOriginals) != 1 {
		t.Fatalf("want exactly 1 mutant with Original \"fmt\" (body only), got %d: %+v", len(fmtOriginals), fmtOriginals)
	}
	if fmtOriginals[0].Operator != opStringEmptyLiterals {
		t.Fatalf("body mutant operator=%q want %q", fmtOriginals[0].Operator, opStringEmptyLiterals)
	}
}

func TestStructTagsAreStillMutated(t *testing.T) {
	// Composition with #375: import skip must not suppress struct-tag mutants.
	src := `package sample

import "fmt"

type Config struct {
	Type string ` + "`yaml:\"type\"`" + `
}

func Use() { _ = fmt.Sprint }
`
	mutants, err := Generate("sample", "sample.go", []byte(src), ProfileDefault)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	tag := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`yaml:\"type\"`")
	if len(tag) != 1 {
		t.Fatalf("want 1 string-empty-literals tag mutant, got %d in %+v", len(tag), mutants)
	}
	assertHighStructTagEquivalent(t, tag[0])

	for _, m := range mutants {
		if m.Original == `"fmt"` {
			t.Fatalf("import path must not be mutated: %+v", m)
		}
	}
}
