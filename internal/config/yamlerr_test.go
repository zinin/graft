package config

import (
	"errors"
	"testing"
)

func TestYAMLError(t *testing.T) {
	tests := []struct {
		name     string
		text     string // what the error says
		wantLine int
		wantMsg  string
	}{
		{"parser message", "yaml: line 8: did not find expected ',' or ']'", 9, "did not find expected ',' or ']'"},
		{"scanner message", "yaml: line 2: found character that cannot start any token", 2, "found character that cannot start any token"},
		{"no line", "yaml: unknown anchor 'x' referenced", 0, "yaml: unknown anchor 'x' referenced"},
		{"parser message without a line", "yaml: found incompatible YAML document", 0, "yaml: found incompatible YAML document"},
		{"several errors", "yaml: unmarshal errors:\n  line 3: cannot unmarshal !!str `a` into int", 0,
			"yaml: unmarshal errors:\n  line 3: cannot unmarshal !!str `a` into int"},
		{"line 0 is no line", "yaml: line 0: did not find expected key", 0, "yaml: line 0: did not find expected key"},
		{"line too big for an int", "yaml: line 99999999999999999999: did not find expected key", 0,
			"yaml: line 99999999999999999999: did not find expected key"},
		{"not from yaml.v3", "line 3: did not find expected key", 0, "line 3: did not find expected key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, msg := YAMLError(errors.New(tt.text))
			if line != tt.wantLine || msg != tt.wantMsg {
				t.Errorf("YAMLError(%q) = %d, %q, want %d, %q", tt.text, line, msg, tt.wantLine, tt.wantMsg)
			}
		})
	}
}

// TestYAMLErrorParserMessages: the line of every message of the yaml.v3
// parser is one less than the line of the file. The messages are typed here
// on their own, so that a slip in the list of YAMLError shows.
func TestYAMLErrorParserMessages(t *testing.T) {
	for _, m := range []string{
		"did not find expected <stream-start>",
		"did not find expected <document start>",
		"did not find expected node content",
		"did not find expected '-' indicator",
		"did not find expected key",
		"did not find expected ',' or ']'",
		"did not find expected ',' or '}'",
		"found duplicate %YAML directive",
		"found duplicate %TAG directive",
		"found incompatible YAML document",
		"found undefined tag handle",
	} {
		if line, msg := YAMLError(errors.New("yaml: line 4: " + m)); line != 5 || msg != m {
			t.Errorf("YAMLError(%q) = %d, %q, want 5, %q", "yaml: line 4: "+m, line, msg, m)
		}
	}
}

// TestParseSyntaxLines: the line of a syntax error is the line to look at,
// and the message comes without the "yaml: line N: " that yaml.v3 puts in
// front of it, whose parser counts lines from 0.
func TestParseSyntaxLines(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantLine int
		wantMsg  string
	}{
		{"unclosed bracket", "providers:\n  x:\n    url: [https://a\n    key: {file: k}\n", 3, "did not find expected ',' or ']'"},
		{"unclosed brace", "providers:\n  x:\n    url: {https://a\n    key: {file: k}\n", 3, "did not find expected ',' or '}'"},
		{"tab at the start of a line", "profiles:\n\tx: {}\n", 2, "found character that cannot start any token"},
		{"second colon in a line", "x: 1\na: b: c\n", 2, "mapping values are not allowed in this context"},
		{"unclosed bracket in the second document",
			"profiles:\n  a: { model: opus }\n---\nproviders:\n  x:\n    url: [https://a\n    key: {file: k}\n",
			6, "did not find expected ',' or ']'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(t, tt.src)
			var errs Errors
			if !errors.As(err, &errs) || len(errs) != 1 {
				t.Fatalf("error %v, want one Error", err)
			}
			if errs[0].Line != tt.wantLine || errs[0].Msg != tt.wantMsg {
				t.Errorf("line %d, message %q, want line %d, message %q", errs[0].Line, errs[0].Msg, tt.wantLine, tt.wantMsg)
			}
		})
	}
}
