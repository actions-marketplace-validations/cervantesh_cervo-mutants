package mutator

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// tagDefaultDerivation maps known encoding keys to the name the decoder would
// derive when the tag is absent. Keys not present here make the whole tag
// non-equivalent (unknown keys may carry independent semantics).
//
// json must match the field name EXACTLY (case-sensitive). encoding/json
// Unmarshal falls back to a case-insensitive match, so json:"type" on field
// Type would decode the same. But Marshal would change the emitted key
// "type" -> "Type". The generator cannot know statically whether a struct is
// only ever unmarshaled, so the predicate MUST require exact case-sensitive
// match. Using strings.EqualFold here would be a BUG.
var tagDefaultDerivation = map[string]func(fieldName string) string{
	"yaml": strings.ToLower,
	"json": func(fieldName string) string { return fieldName },
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
		if _, known := tagDefaultDerivation[key]; !known {
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
		// Empty name already means "use default" for that key.
		if name == "" {
			continue
		}
		derive := tagDefaultDerivation[key]
		if name != derive(fieldName) {
			return false
		}
	}
	return true
}
