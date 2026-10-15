package config

import (
	"errors"
	"strings"
	"testing"
)

// nLines returns a text of n lines, each ending with a line feed.
func nLines(n int) string { return strings.Repeat("a: 1\n", n) }

func TestYAMLError(t *testing.T) {
	tests := []struct {
		name     string
		text     string // what the error says
		data     string // what the decoder was reading
		wantLine int
		wantMsg  string
	}{
		{"parser message", "yaml: line 8: did not find expected ',' or ']'", nLines(12), 9, "did not find expected ',' or ']'"},
		{"scanner message", "yaml: line 2: found character that cannot start any token", nLines(12), 2, "found character that cannot start any token"},
		{"no line", "yaml: unknown anchor 'x' referenced", nLines(12), 0, "yaml: unknown anchor 'x' referenced"},
		{"parser message without a line", "yaml: found incompatible YAML document", nLines(12), 0, "yaml: found incompatible YAML document"},
		{"several errors", "yaml: unmarshal errors:\n  line 3: cannot unmarshal !!str `a` into int", nLines(12), 0,
			"yaml: unmarshal errors:\n  line 3: cannot unmarshal !!str `a` into int"},
		{"line 0 is no line", "yaml: line 0: did not find expected key", nLines(12), 0, "yaml: line 0: did not find expected key"},
		{"line too big for an int", "yaml: line 99999999999999999999: did not find expected key", nLines(12), 0,
			"yaml: line 99999999999999999999: did not find expected key"},
		{"not from yaml.v3", "line 3: did not find expected key", nLines(12), 0, "line 3: did not find expected key"},

		// yaml.v3 puts the end of the stream on the line after the last one, so
		// an error found at the end names a line the text does not have.
		{"past the end", "yaml: line 2: did not find expected node content", "a: 1\nb: [\n", 2, "did not find expected node content"},
		{"past the end, no final line feed", "yaml: line 2: did not find expected node content", "a: 1\nb: [", 2, "did not find expected node content"},
		{"past the end, CRLF", "yaml: line 2: did not find expected node content", "a: 1\r\nb: [\r\n", 2, "did not find expected node content"},
		{"past the end, one line", "yaml: line 1: did not find expected node content", "profiles: [", 1, "did not find expected node content"},
		{"scanner message past the end", "yaml: line 5: found unexpected end of stream", "a: 1\nb: \"\n", 2, "found unexpected end of stream"},
		{"the last line stays", "yaml: line 1: did not find expected ',' or ']'", "a: 1\nb: [\n", 2, "did not find expected ',' or ']'"},
		{"a line inside the data stays", "yaml: line 1: did not find expected ',' or ']'", "a: 1\nb: [\nc: 2\n", 2, "did not find expected ',' or ']'"},
		{"no line in empty data", "yaml: line 2: did not find expected node content", "", 0, "yaml: line 2: did not find expected node content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, msg := YAMLError(errors.New(tt.text), []byte(tt.data))
			if line != tt.wantLine || msg != tt.wantMsg {
				t.Errorf("YAMLError(%q, %q) = %d, %q, want %d, %q", tt.text, tt.data, line, msg, tt.wantLine, tt.wantMsg)
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
		if line, msg := YAMLError(errors.New("yaml: line 4: "+m), []byte(nLines(5))); line != 5 || msg != m {
			t.Errorf("YAMLError(%q) = %d, %q, want 5, %q", "yaml: line 4: "+m, line, msg, m)
		}
	}
}

// TestParseSyntaxLines: the line of a syntax error is the line to look at,
// and the message comes without the "yaml: line N: " that yaml.v3 puts in
// front of it, whose parser counts lines from 0. An error found at the end of
// the text is on its last line, which the file has.
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

		{"unclosed bracket at the end", "providers: [\n", 1, "did not find expected node content"},
		{"unclosed bracket at the end, no final line feed", "profiles: [", 1, "did not find expected node content"},
		{"unclosed bracket at the end of the second document", "x: 1\n---\nb: [\n", 3, "did not find expected node content"},
		// A [ on the first line is not the context of the error to yaml.v3: it
		// names the line where it noticed the problem, which the file has.
		{"unclosed bracket on the first line", "x: [1, 2\ny: 3\n", 2, "did not find expected ',' or ']'"},
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
