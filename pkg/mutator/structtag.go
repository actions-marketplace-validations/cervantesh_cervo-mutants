package mutator

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// tagKnownKeys lists the encoding keys whose absent-tag behavior this predicate
// can reason about at all. A tag carrying any other key is never treated as
// equivalent, because the unknown key may have independent semantics.
var tagKnownKeys = map[string]struct{}{
	"yaml": {},
	"json": {},
}

// tagDefaultDerivation maps the subset of known keys for which a NON-EMPTY tag
// name can still be provably equivalent, to the name the decoder derives when
// the tag is absent.
//
// yaml qualifies: gopkg.in/yaml.v3 derives strings.ToLower(field.Name) when the
// tag is absent, and it has no cross-field dominance rule, so a name-for-name
// match is genuinely behavior-preserving.
//
// json deliberately does NOT qualify, and is absent from this map. encoding/json
// derives the field name exactly, but it also resolves same-name collisions by
// depth and then by whether the field carries a tag. Emptying a json tag flips
// that "tagged" flag, which can change which field wins, or drop both:
//
//	type T struct {
//	    Name  string `json:"Name"`
//	    Other string `json:"Name"`
//	}
//
// Marshal yields {} before the mutation and {"Name":...} after. The colliding
// field may even live in a different type declaration, reached by equal-depth
// embedding, so no field-local analysis can ever see it. A non-empty json name
// is therefore never provably equivalent, however exactly it matches. json
// stays in tagKnownKeys so a json-tagged UNEXPORTED field still qualifies
// through the inert-field shortcut, where encoding/json never even enters the
// field into its collision resolution.
var tagDefaultDerivation = map[string]func(fieldName string) string{
	"yaml": strings.ToLower,
}

// structTagField reports whether node is the struct-field tag BasicLit of its
// parent Field. The field.Tag == lit identity check is load-bearing: a string
// BasicLit may appear elsewhere under a Field (or nearby) without being the tag.
func structTagField(parents map[ast.Node]ast.Node, node ast.Node) (*ast.Field, bool) {
	lit, ok := node.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return nil, false
	}
	field, ok := parents[lit].(*ast.Field)
	if !ok || field.Tag != lit {
		return nil, false
	}
	return field, true
}

// parseStructTagPairs strictly re-implements the reflect.StructTag grammar and
// enumerates every key:"value" pair. Unlike reflect.StructTag.Get/Lookup, this
// parser never silently tolerates garbage: any malformed pair, duplicate key,
// or trailing junk (including trailing spaces) yields ok=false.
func parseStructTagPairs(tag string) (map[string]string, bool) {
	// Trailing space is trailing junk for this strict predicate.
	if len(tag) > 0 && tag[len(tag)-1] == ' ' {
		return nil, false
	}

	pairs := make(map[string]string)
	n := 0
	for tag != "" {
		// Pairs must be space-separated (more restrictive than reflect, matches go vet).
		if n > 0 && tag[0] != ' ' {
			return nil, false
		}
		// Skip leading space.
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		if tag == "" {
			break
		}

		// Scan to colon. A space, a quote or a control character is a syntax error.
		i = 0
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' && tag[i] != 0x7f {
			i++
		}
		if i == 0 || i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			return nil, false
		}
		key := string(tag[:i])
		tag = tag[i+1:]

		// Scan quoted string to find value.
		i = 1
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++ // skip escaped character
			}
			i++
		}
		if i >= len(tag) {
			return nil, false
		}
		qvalue := string(tag[:i+1])
		tag = tag[i+1:]

		value, err := strconv.Unquote(qvalue)
		if err != nil {
			return nil, false
		}
		if _, dup := pairs[key]; dup {
			return nil, false
		}
		pairs[key] = value
		n++
	}
	return pairs, true
}

// isProvablyEquivalentTagMutation reports whether emptying the whole field tag
// is provably behavior-preserving for known reflect-based encoders.
func isProvablyEquivalentTagMutation(field *ast.Field) bool {
	if field == nil || field.Tag == nil || len(field.Names) != 1 {
		return false
	}
	fieldName := field.Names[0].Name

	unquoted, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return false
	}
	pairs, ok := parseStructTagPairs(unquoted)
	if !ok || len(pairs) == 0 {
		return false
	}

	// Unknown-key gate: any non-derivable key makes the whole tag non-equivalent.
	// This must run before the unexported-field shortcut.
	for key := range pairs {
		if _, known := tagKnownKeys[key]; !known {
			return false
		}
	}

	// Unexported fields are ignored by reflect-based encoders (json/yaml), so
	// any tag composed only of known keys is inert and emptying it is equivalent.
	if !ast.IsExported(fieldName) {
		return true
	}

	// Multi-key tags: emptying drops ALL keys, so every key must pass.
	for key, value := range pairs {
		// Options after a comma (omitempty, inline, flow, string, even json:"-,")
		// are dropped when the tag is emptied → never equivalent.
		if strings.Contains(value, ",") {
			return false
		}
		name := value
		// Tag suppresses the field; emptying resurrects it.
		if name == "-" {
			return false
		}
		// Empty name already means "use default" for that key, and it leaves
		// encoding/json's "tagged" flag false both before and after.
		if name == "" {
			continue
		}
		// A known key with no derivation entry (json) can never prove a
		// non-empty name equivalent. See tagDefaultDerivation.
		derive, derivable := tagDefaultDerivation[key]
		if !derivable {
			return false
		}
		if name != derive(fieldName) {
			return false
		}
	}
	return true
}
