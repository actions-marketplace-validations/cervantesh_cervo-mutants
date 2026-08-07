package mutator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestYamlTagMatchingDefaultDerivationIsHighEquivalentRisk(t *testing.T) {
	src := `package sample

type Config struct {
	Type string ` + "`yaml:\"type\"`" + `
}
`
	// Default profile emits string-empty-literals; aggressive also co-emits literals.
	for _, profile := range []string{ProfileDefault, ProfileAggressive} {
		mutants, err := Generate("sample", "sample.go", []byte(src), profile)
		if err != nil {
			t.Fatalf("Generate(%s): %v", profile, err)
		}
		empty := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`yaml:\"type\"`")
		if len(empty) != 1 {
			t.Fatalf("profile %s: want 1 string-empty-literals tag mutant, got %d in %+v", profile, len(empty), mutants)
		}
		assertHighStructTagEquivalent(t, empty[0])

		if profile == ProfileAggressive {
			lit := tagMutantsForOriginal(mutants, opLiterals, "`yaml:\"type\"`")
			if len(lit) != 1 {
				t.Fatalf("aggressive: want 1 literals tag mutant, got %d", len(lit))
			}
			assertHighStructTagEquivalent(t, lit[0])
		}
	}
}

func TestYamlTagDivergingFromDefaultStaysMedium(t *testing.T) {
	// yaml.v3 default for GoName is "goname"; "go_name" is a deliberate rename.
	src := `package sample

type Config struct {
	GoName string ` + "`yaml:\"go_name\"`" + `
}
`
	mutants, err := Generate("sample", "sample.go", []byte(src), ProfileDefault)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`yaml:\"go_name\"`")
	if len(found) != 1 {
		t.Fatalf("want 1 mutant, got %d: %+v", len(found), mutants)
	}
	assertMediumNoStructTagEquivalent(t, found[0])
}

func TestTagOptionsAfterCommaStayMedium(t *testing.T) {
	src := `package sample

type Config struct {
	Type string ` + "`yaml:\"type,omitempty\"`" + `
	Kind string ` + "`json:\"Kind,string\"`" + `
}
`
	mutants, err := Generate("sample", "sample.go", []byte(src), ProfileDefault)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, original := range []string{"`yaml:\"type,omitempty\"`", "`json:\"Kind,string\"`"} {
		found := tagMutantsForOriginal(mutants, opStringEmptyLiterals, original)
		if len(found) != 1 {
			t.Fatalf("want 1 mutant for %s, got %d: %+v", original, len(found), mutants)
		}
		assertMediumNoStructTagEquivalent(t, found[0])
	}
}

func TestJsonExactCaseSensitiveDerivation(t *testing.T) {
	// encoding/json Unmarshal falls back to case-insensitive matching, so
	// json:"type" on field Type would still decode. Marshal would emit "Type"
	// instead of "type", so only the exact case-sensitive match is high.
	src := `package sample

type Exact struct {
	Type string ` + "`json:\"Type\"`" + `
}

type Fold struct {
	Type string ` + "`json:\"type\"`" + `
}
`
	mutants, err := Generate("sample", "sample.go", []byte(src), ProfileDefault)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	exact := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`json:\"Type\"`")
	if len(exact) != 1 {
		t.Fatalf("exact: want 1 mutant, got %d: %+v", len(exact), mutants)
	}
	assertHighStructTagEquivalent(t, exact[0])

	folded := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`json:\"type\"`")
	if len(folded) != 1 {
		t.Fatalf("folded: want 1 mutant, got %d: %+v", len(folded), mutants)
	}
	assertMediumNoStructTagEquivalent(t, folded[0])
}

func TestMultiKeyTagsRequireEveryKeyToMatch(t *testing.T) {
	src := `package sample

type AllMatch struct {
	Type string ` + "`json:\"Type\" yaml:\"type\"`" + `
}

type YamlDiverges struct {
	Type string ` + "`json:\"Type\" yaml:\"go_name\"`" + `
}

type UnknownKey struct {
	Type string ` + "`json:\"Type\" validate:\"required\"`" + `
}
`
	mutants, err := Generate("sample", "sample.go", []byte(src), ProfileDefault)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	allMatch := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`json:\"Type\" yaml:\"type\"`")
	if len(allMatch) != 1 {
		t.Fatalf("allMatch: want 1, got %d: %+v", len(allMatch), mutants)
	}
	assertHighStructTagEquivalent(t, allMatch[0])

	yamlDiv := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`json:\"Type\" yaml:\"go_name\"`")
	if len(yamlDiv) != 1 {
		t.Fatalf("yamlDiv: want 1, got %d", len(yamlDiv))
	}
	assertMediumNoStructTagEquivalent(t, yamlDiv[0])

	unknown := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`json:\"Type\" validate:\"required\"`")
	if len(unknown) != 1 {
		t.Fatalf("unknown: want 1, got %d", len(unknown))
	}
	assertMediumNoStructTagEquivalent(t, unknown[0])
}

func TestSuppressedMalformedAndEmbeddedTagsStayMedium(t *testing.T) {
	src := `package sample

type Config struct {
	Type string ` + "`json:\"-\"`" + `
	Bad  string ` + "`json:\"Type\" yaml`" + `
	Inner ` + "`json:\"Inner\"`" + `
}

type Inner struct {
	X int
}
`
	mutants, err := Generate("sample", "sample.go", []byte(src), ProfileDefault)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	suppressed := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`json:\"-\"`")
	if len(suppressed) != 1 {
		t.Fatalf("suppressed: want 1, got %d: %+v", len(suppressed), mutants)
	}
	assertMediumNoStructTagEquivalent(t, suppressed[0])

	malformed := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`json:\"Type\" yaml`")
	if len(malformed) != 1 {
		t.Fatalf("malformed: want 1, got %d: %+v", len(malformed), mutants)
	}
	assertMediumNoStructTagEquivalent(t, malformed[0])

	// Embedded fields have len(Names)==0; even a matching-looking tag must stay medium.
	embedded := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`json:\"Inner\"`")
	if len(embedded) != 1 {
		t.Fatalf("embedded: want 1, got %d: %+v", len(embedded), mutants)
	}
	assertMediumNoStructTagEquivalent(t, embedded[0])
}

func TestUnexportedFieldTagsAreInertForKnownEncoders(t *testing.T) {
	src := `package sample

type Config struct {
	hidden string ` + "`yaml:\"anything\"`" + `
	secret string ` + "`db:\"x\"`" + `
}
`
	mutants, err := Generate("sample", "sample.go", []byte(src), ProfileDefault)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	known := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`yaml:\"anything\"`")
	if len(known) != 1 {
		t.Fatalf("known: want 1, got %d: %+v", len(known), mutants)
	}
	assertHighStructTagEquivalent(t, known[0])

	unknown := tagMutantsForOriginal(mutants, opStringEmptyLiterals, "`db:\"x\"`")
	if len(unknown) != 1 {
		t.Fatalf("unknown: want 1, got %d: %+v", len(unknown), mutants)
	}
	assertMediumNoStructTagEquivalent(t, unknown[0])
}

func TestPlainStringLiteralInsideFunctionStaysMediumWithoutTags(t *testing.T) {
	src := `package sample

func Message() string {
	return "hello"
}
`
	mutants, err := Generate("sample", "sample.go", []byte(src), ProfileDefault)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := tagMutantsForOriginal(mutants, opStringEmptyLiterals, `"hello"`)
	if len(found) != 1 {
		t.Fatalf("want 1 mutant for plain string, got %d: %+v", len(found), mutants)
	}
	m := found[0]
	if m.EquivalentRisk != "medium" {
		t.Fatalf("plain string EquivalentRisk=%q want medium", m.EquivalentRisk)
	}
	if containsTag(m.SemanticTags, "struct-tag-equivalent") {
		t.Fatalf("plain string must not gain struct-tag-equivalent: %+v", m)
	}
	if m.SuggestedSkipReason != "" {
		t.Fatalf("plain string SuggestedSkipReason=%q want empty", m.SuggestedSkipReason)
	}
}

func TestStrictStructTagParserEdgeCases(t *testing.T) {
	// White-box predicate checks for parser strictness.
	cases := []struct {
		name   string
		src    string
		wantEq bool
	}{
		{
			name:   "duplicate keys",
			src:    "package sample\ntype T struct { Type string `json:\"Type\" json:\"type\"` }\n",
			wantEq: false,
		},
		{
			name:   "trailing space",
			src:    "package sample\ntype T struct { Type string `json:\"Type\" ` }\n",
			wantEq: false,
		},
		{
			name:   "json dash-comma options",
			src:    "package sample\ntype T struct { Type string `json:\"-,\"` }\n",
			wantEq: false,
		},
		{
			name:   "exact json still equivalent",
			src:    "package sample\ntype T struct { Type string `json:\"Type\"` }\n",
			wantEq: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			field := parseFirstStructField(t, tc.src)
			got := isProvablyEquivalentTagMutation(field)
			if got != tc.wantEq {
				t.Fatalf("isProvablyEquivalentTagMutation=%v want %v for tag %s", got, tc.wantEq, field.Tag.Value)
			}
		})
	}
}

func TestStructTagFieldRequiresTagIdentity(t *testing.T) {
	src := `package sample

type T struct {
	Type string ` + "`json:\"Type\"`" + `
}

func Message() string { return "Type" }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", src, 0)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	parents := buildParentIndex(file)

	var tagLit, bodyLit *ast.BasicLit
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if field, ok := parents[lit].(*ast.Field); ok && field.Tag == lit {
			tagLit = lit
		} else if lit.Value == `"Type"` {
			bodyLit = lit
		}
		return true
	})
	if tagLit == nil || bodyLit == nil {
		t.Fatalf("failed to locate tag and body literals")
	}
	if _, ok := structTagField(parents, tagLit); !ok {
		t.Fatal("structTagField should match field tag")
	}
	if _, ok := structTagField(parents, bodyLit); ok {
		t.Fatal("structTagField must not match non-tag string literal")
	}
}

func assertHighStructTagEquivalent(t *testing.T, m Mutant) {
	t.Helper()
	if m.EquivalentRisk != "high" {
		t.Fatalf("EquivalentRisk=%q want high for %+v", m.EquivalentRisk, m)
	}
	if !containsTag(m.SemanticTags, "struct-tag-equivalent") {
		t.Fatalf("missing struct-tag-equivalent tag: %+v", m)
	}
	wantReason := "tag name matches the decoder's default derivation; emptying it cannot change behavior"
	if m.SuggestedSkipReason != wantReason {
		t.Fatalf("SuggestedSkipReason=%q want %q", m.SuggestedSkipReason, wantReason)
	}
}

func assertMediumNoStructTagEquivalent(t *testing.T, m Mutant) {
	t.Helper()
	if m.EquivalentRisk != "medium" {
		t.Fatalf("EquivalentRisk=%q want medium for %+v", m.EquivalentRisk, m)
	}
	if containsTag(m.SemanticTags, "struct-tag-equivalent") {
		t.Fatalf("unexpected struct-tag-equivalent tag: %+v", m)
	}
}

func tagMutantsForOriginal(mutants []Mutant, operator, original string) []Mutant {
	var out []Mutant
	for _, m := range mutants {
		if m.Operator == operator && m.Original == original {
			out = append(out, m)
		}
	}
	return out
}

func parseFirstStructField(t *testing.T, src string) *ast.Field {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", src, 0)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var field *ast.Field
	ast.Inspect(file, func(n ast.Node) bool {
		if field != nil {
			return false
		}
		if f, ok := n.(*ast.Field); ok && f.Tag != nil && len(f.Names) == 1 {
			field = f
			return false
		}
		return true
	})
	if field == nil {
		t.Fatal("no tagged field found")
	}
	return field
}
