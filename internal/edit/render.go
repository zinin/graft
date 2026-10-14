package edit

import (
	"bytes"

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

// snippet returns n as YAML text, as a settings field shows it: without
// the anchor that aliases of it use.
func snippet(n *yaml.Node) (string, error) {
	c := *deref(n)
	c.Anchor = ""
	data, err := encode(&c)
	return string(data), err
}
