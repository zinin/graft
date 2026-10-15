package edit

import (
	"bytes"
	"fmt"
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

// snippet returns n as YAML text, as a settings field shows it: on its own,
// with a copy of what each alias in it points at and no anchor, because the
// text goes where the rest of the file is out of sight.
func snippet(n *yaml.Node) (string, error) {
	c, err := expand(n)
	if err != nil {
		return "", err
	}
	data, err := encode(c)
	return string(data), err
}

// maxExpanded is the most nodes a settings value may have once its aliases
// are copies. Real settings have a few hundred, so the limit is far above
// them. It is there for an alias bomb, a few lines of aliases of aliases
// that would copy into billions of nodes and fill the memory: the copy of
// one stops at the limit, within some milliseconds and about 20 MB.
const maxExpanded = 100_000

// expand returns a copy of n that holds no alias and no anchor: each alias
// is replaced by a copy of the node it points at, so the copy means the
// same wherever it is put. Comments and styles stay; the copy of an alias
// has the comments written on the alias, and those inside what it points
// at. It fails for an alias inside the node it points at, which would never
// end, and for a copy of more than maxExpanded nodes.
func expand(n *yaml.Node) (*yaml.Node, error) {
	e := &expansion{left: maxExpanded, open: map[*yaml.Node]bool{}}
	return e.node(n)
}

// expansion is one run of expand.
type expansion struct {
	left int                 // how many more nodes the copy may have
	open map[*yaml.Node]bool // the nodes on the way from the root to the one being copied
}

// node returns the copy of n, with the copies of the nodes under it.
func (e *expansion) node(n *yaml.Node) (*yaml.Node, error) {
	if n.Kind == yaml.AliasNode {
		if e.open[n.Alias] {
			return nil, fmt.Errorf("recursive alias *%s: it points at a value that contains it", n.Value)
		}
		c, err := e.node(n.Alias)
		if err != nil {
			return nil, err
		}
		// The comments on the node it points at describe the place of that
		// node; those on the alias describe this place.
		c.HeadComment, c.LineComment, c.FootComment = n.HeadComment, n.LineComment, n.FootComment
		return c, nil
	}
	if e.left--; e.left < 0 {
		return nil, fmt.Errorf("the aliases expand to more than %d nodes", maxExpanded)
	}
	e.open[n] = true
	defer delete(e.open, n)
	c := *n
	c.Anchor, c.Content = "", nil
	for _, child := range n.Content {
		cc, err := e.node(child)
		if err != nil {
			return nil, err
		}
		c.Content = append(c.Content, cc)
	}
	return &c, nil
}
