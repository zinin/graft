package edit

import (
	"bytes"
	"strings"

	"go.yaml.in/yaml/v3"
)

// encode writes n as YAML with an indent of 2.
func encode(n *yaml.Node) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// render writes the document with a blank line before every top-level key
// but the first, so that the sections stand apart.
func render(doc *yaml.Node) ([]byte, error) {
	data, err := encode(doc)
	if err != nil {
		return nil, err
	}
	return spaceSections(data), nil
}

// spaceSections puts a blank line before each top-level key but the first,
// and before the comment lines right above such a key.
func spaceSections(data []byte) []byte {
	lines := strings.SplitAfter(string(data), "\n")
	seenKey := false
	for i, line := range lines {
		if !isTopKey(line) {
			continue
		}
		if seenKey {
			// The key and the comment lines right above it.
			start := i
			for start > 0 && strings.HasPrefix(lines[start-1], "#") {
				start--
			}
			if start > 0 && strings.TrimSpace(lines[start-1]) != "" {
				lines[start] = "\n" + lines[start]
			}
		}
		seenKey = true
	}
	return []byte(strings.Join(lines, ""))
}

// isTopKey reports whether line starts a key of the top-level mapping.
func isTopKey(line string) bool {
	if line == "" || line == "\n" {
		return false
	}
	switch line[0] {
	case ' ', '\t', '#', '-', '\n', '{', '[', '}', ']', '.':
		return false
	}
	return true
}

// snippet returns n as YAML text, as a settings field shows it: without
// the anchor that aliases of it use.
func snippet(n *yaml.Node) (string, error) {
	c := *deref(n)
	c.Anchor = ""
	data, err := encode(&c)
	return string(data), err
}
