package edit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/zinin/graft/internal/config"
)

// ShapeError says why the forms cannot show a config file: the file is not
// one YAML document, or a part of it has a shape the draft cannot hold.
type ShapeError struct {
	Problems []Problem
}

// Problem is one reason, at its line of the file when known.
type Problem struct {
	Line int    `json:"line"`
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

func (e *ShapeError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		s := ""
		if p.Line > 0 {
			s = fmt.Sprintf("line %d: ", p.Line)
		}
		if p.Path != "" {
			s += p.Path + ": "
		}
		lines[i] = s + p.Msg
	}
	return strings.Join(lines, "\n")
}

// Read returns the draft of the config in data; empty data is an empty
// config. It returns a *ShapeError when the forms cannot show the file.
func Read(data []byte) (Draft, error) {
	doc, err := parse(data)
	if err != nil {
		return Draft{}, err
	}
	r := &reader{}
	d := r.draft(doc.Content[0])
	if len(r.problems) > 0 {
		return Draft{}, &ShapeError{r.problems}
	}
	d.Normalize()
	return d, nil
}

// parse returns the document in data. Its root is a mapping when the file
// is empty or holds only comments.
func parse(data []byte) (*yaml.Node, error) {
	doc, err := decode(data)
	if err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	} else if root := doc.Content[0]; isNull(root) {
		doc.Content[0] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map",
			HeadComment: root.HeadComment, LineComment: root.LineComment, FootComment: root.FootComment}
	}
	return doc, nil
}

// decode returns the one YAML document in data; its Content is empty when
// data holds no value.
func decode(data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc, next yaml.Node
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, &ShapeError{[]Problem{{Msg: err.Error()}}}
	}
	switch err := dec.Decode(&next); {
	case err == nil:
		return nil, &ShapeError{[]Problem{{Line: next.Line, Msg: "must be one YAML document"}}}
	case !errors.Is(err, io.EOF):
		return nil, &ShapeError{[]Problem{{Msg: err.Error()}}}
	}
	doc.Kind = yaml.DocumentNode
	return &doc, nil
}

type reader struct {
	problems []Problem
}

func (r *reader) problem(n *yaml.Node, path, format string, args ...any) {
	line := 0
	if n != nil {
		line = n.Line
	}
	r.problems = append(r.problems, Problem{Line: line, Path: path, Msg: fmt.Sprintf(format, args...)})
}

// pair is one key and value of a mapping, the value with aliases followed.
type pair struct {
	key   string
	value *yaml.Node
}

// pairs returns the pairs of mapping n in file order; null is no pairs.
// Keys must be strings and appear once: the draft could not tell two
// apart. A key is not an alias either: Apply finds an entry by the text of
// its key node, and an alias holds the name of its anchor instead.
func (r *reader) pairs(n *yaml.Node, path string) []pair {
	n = deref(n)
	if n == nil || isNull(n) {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		r.problem(n, path, "must be a mapping")
		return nil
	}
	var out []pair
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		switch {
		case k.Kind == yaml.AliasNode:
			r.problem(k, path, "a key must not be an alias (*%s): write the name itself", k.Value)
		case k.Kind != yaml.ScalarNode:
			r.problem(k, path, "keys must be strings")
		case seen[k.Value]:
			r.problem(k, join(path, k.Value), "duplicate key")
		default:
			seen[k.Value] = true
			out = append(out, pair{k.Value, deref(n.Content[i+1])})
		}
	}
	return out
}

// fields returns the values of mapping n by key.
func (r *reader) fields(n *yaml.Node, path string) map[string]*yaml.Node {
	out := map[string]*yaml.Node{}
	for _, p := range r.pairs(n, path) {
		out[p.key] = p.value
	}
	return out
}

// scalar returns a value as written; a missing or null value is "".
func (r *reader) scalar(n *yaml.Node, path string) string {
	switch {
	case n == nil || isNull(n):
		return ""
	case n.Kind != yaml.ScalarNode:
		r.problem(n, path, "must be a single value")
		return ""
	}
	return n.Value
}

// list returns the items of sequence n; null is an empty list.
func (r *reader) list(n *yaml.Node, path string) []string {
	if n == nil || isNull(n) {
		return []string{}
	}
	if n.Kind != yaml.SequenceNode {
		r.problem(n, path, "must be a list")
		return []string{}
	}
	out := []string{}
	for i, item := range n.Content {
		out = append(out, r.scalar(deref(item), fmt.Sprintf("%s[%d]", path, i)))
	}
	return out
}

// env returns the variables of mapping n in file order.
func (r *reader) env(n *yaml.Node, path string) []Pair {
	out := []Pair{}
	for _, p := range r.pairs(n, path) {
		out = append(out, Pair{p.key, r.scalar(p.value, join(path, p.key))})
	}
	return out
}

// settings returns the value of n as YAML text; a missing or null value is
// "". Any value can be text, so settings never limit the forms.
func (r *reader) settings(n *yaml.Node, path string) string {
	if n == nil || isNull(n) {
		return ""
	}
	text, err := snippet(n)
	if err != nil {
		r.problem(n, path, "%v", err)
	}
	return text
}

func (r *reader) draft(root *yaml.Node) Draft {
	var d Draft
	f := r.fields(root, "")
	for _, p := range r.pairs(f["providers"], "providers") {
		d.Providers = append(d.Providers, r.provider(p, "providers."+p.key))
	}
	for _, p := range r.pairs(f["models"], "models") {
		d.Models = append(d.Models, r.model(p, "models."+p.key))
	}
	df := r.entry(f["defaults"], "defaults")
	d.Defaults = Defaults{
		Effort:   r.scalar(df["effort"], "defaults.effort"),
		Settings: r.settings(df["settings"], "defaults.settings"),
		Env:      r.env(df["env"], "defaults.env"),
		Args:     r.list(df["args"], "defaults.args"),
	}
	for _, p := range r.pairs(f["profiles"], "profiles") {
		d.Profiles = append(d.Profiles, r.profile(p, "profiles."+p.key))
	}
	return d
}

// entry returns the fields of an entry, which is a mapping or null.
func (r *reader) entry(n *yaml.Node, path string) map[string]*yaml.Node {
	return r.fields(n, path)
}

func (r *reader) provider(p pair, path string) Provider {
	f := r.entry(p.value, path)
	return Provider{
		Was:  p.key,
		Name: p.key,
		URL:  r.scalar(f["url"], path+".url"),
		Key:  r.key(f["key"], path+".key"),
	}
}

// key reads a key source. A key written into the config itself, a scalar,
// is no source: its value must not reach the browser.
func (r *reader) key(n *yaml.Node, path string) Key {
	if n == nil || n.Kind == yaml.ScalarNode {
		return Key{}
	}
	f := r.fields(n, path)
	var k Key
	for _, kind := range config.KeyFields {
		if v, ok := f[kind]; ok {
			value := r.scalar(v, path+"."+kind)
			if k.Kind == "" {
				k = Key{Kind: kind, Value: value}
			}
		}
	}
	return k
}

func (r *reader) model(p pair, path string) Model {
	f := r.entry(p.value, path)
	return Model{
		Was:         p.key,
		Name:        p.key,
		Provider:    r.scalar(f["provider"], path+".provider"),
		ID:          r.scalar(f["id"], path+".id"),
		Context:     r.scalar(f["context"], path+".context"),
		Label:       r.scalar(f["label"], path+".label"),
		Description: r.scalar(f["description"], path+".description"),
		Settings:    r.settings(f["settings"], path+".settings"),
	}
}

func (r *reader) profile(p pair, path string) Profile {
	f := r.entry(p.value, path)
	aliases := map[string]string{}
	for _, a := range r.pairs(f["aliases"], path+".aliases") {
		if slices.Contains(config.AliasNames, a.key) {
			aliases[a.key] = r.scalar(a.value, path+".aliases."+a.key)
		}
	}
	return Profile{
		Was:          p.key,
		Name:         p.key,
		Model:        r.scalar(f["model"], path+".model"),
		Subscription: r.scalar(f["subscription"], path+".subscription"),
		Picker:       r.list(f["picker"], path+".picker"),
		Aliases:      aliases,
		Subagents:    r.scalar(f["subagents"], path+".subagents"),
		Effort:       r.scalar(f["effort"], path+".effort"),
		Settings:     r.settings(f["settings"], path+".settings"),
		Env:          r.env(f["env"], path+".env"),
		Args:         r.list(f["args"], path+".args"),
	}
}

// deref follows YAML aliases.
func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
