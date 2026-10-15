package edit

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/zinin/graft/internal/config"
)

// Error is a problem of a draft at a path of the config, such as
// profiles.mix.settings.
type Error struct {
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

// Errors is every problem of a draft that Apply found.
type Errors []Error

func (es Errors) Error() string {
	lines := make([]string, len(es))
	for i, e := range es {
		lines[i] = e.Path + ": " + e.Msg
	}
	return strings.Join(lines, "\n")
}

// Apply returns data with the changes from Read(data) to d made: entries
// added, removed and renamed by Was, fields set and cleared. Nodes the
// changes do not touch keep their comments, style and anchors. A change
// never reaches another field through a YAML alias: before a value with an
// anchor changes, each alias of it becomes a copy of the old value. A
// settings text is written without anchors and aliases, as Read shows it,
// so it cannot take the place of an anchor of the file. Apply returns
// Errors when d does not fit data, such as an entry whose Was the file
// lacks, or a settings text that is not YAML.
func Apply(data []byte, d Draft) ([]byte, error) {
	base, err := Read(data)
	if err != nil {
		return nil, err
	}
	doc, err := parse(data)
	if err != nil {
		return nil, err
	}
	d.Normalize()
	a := &applier{root: doc.Content[0]}
	entries(a, "providers", base.Providers, d.Providers, a.provider)
	entries(a, "models", base.Models, d.Models, a.model)
	a.defaults(base.Defaults, d.Defaults)
	entries(a, "profiles", base.Profiles, d.Profiles, a.profile)
	if len(a.errs) > 0 {
		return nil, a.errs
	}
	return render(doc)
}

type applier struct {
	root *yaml.Node
	errs Errors
}

func (a *applier) errorf(path, format string, args ...any) {
	a.errs = append(a.errs, Error{Path: path, Msg: fmt.Sprintf(format, args...)})
}

// entry is what entries needs of Provider, Model and Profile.
type entry[T any] interface {
	names() (was, name string)
	body() T // the entry without Was and Name
}

func (p Provider) names() (string, string) { return p.Was, p.Name }
func (m Model) names() (string, string)    { return m.Was, m.Name }
func (p Profile) names() (string, string)  { return p.Was, p.Name }

func (p Provider) body() Provider { p.Was, p.Name = "", ""; return p }
func (m Model) body() Model       { m.Was, m.Name = "", ""; return m }
func (p Profile) body() Profile   { p.Was, p.Name = "", ""; return p }

// entries applies the entries of a section: it removes the entries of base
// that want lacks, renames the rest, fills those whose fields changed and
// adds the new ones. fill sets the fields of an entry's mapping from old,
// nil for a new entry, to cur.
func entries[T entry[T]](a *applier, section string, base, want []T, fill func(m *yaml.Node, old *T, cur T, path string)) {
	olds := map[string]*T{}
	for i := range base {
		was, _ := base[i].names()
		olds[was] = &base[i]
	}
	used := map[string]bool{}
	for _, e := range want {
		was, _ := e.names()
		switch {
		case was == "":
		case olds[was] == nil:
			a.errorf(section, "the file has no entry %q: reload the page", was)
			return
		case used[was]:
			a.errorf(section, "entry %q appears twice", was)
			return
		default:
			used[was] = true
		}
	}
	if len(base) == 0 && len(want) == 0 {
		return
	}
	sec := a.mapping(a.root, section, config.Sections, 0)
	// Keys by Was before any rename, so that two entries can swap names.
	keys := map[string]*yaml.Node{}
	for was := range olds {
		i := find(sec, was)
		if !used[was] {
			a.drop(sec, i)
			continue
		}
		keys[was] = sec.Content[i]
	}
	for _, e := range want {
		was, name := e.names()
		path := section + "." + name
		if was == "" {
			m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			fill(m, nil, e, path)
			sec.Content = append(sec.Content, scalarNode(name, "!!str"), m)
			continue
		}
		k := keys[was]
		if k.Value != name {
			// The rename must not reach the aliases of the key.
			a.unshare(k)
			k.Value = name
		}
		if old := olds[was]; !reflect.DeepEqual((*old).body(), e.body()) {
			fill(a.own(sec, slices.Index(sec.Content, k)+1), old, e, path)
		}
	}
	if len(sec.Content) == 0 && len(base) > 0 {
		a.drop(a.root, find(a.root, section))
	}
}

func (a *applier) provider(m *yaml.Node, old *Provider, cur Provider, path string) {
	var o Provider
	if old != nil {
		o = *old
	}
	a.scalar(m, "url", config.ProviderFields, o.URL, cur.URL, strTag)
	a.key(m, o.Key, cur.Key)
}

func (a *applier) model(m *yaml.Node, old *Model, cur Model, path string) {
	var o Model
	if old != nil {
		o = *old
	}
	f := config.ModelFields
	a.scalar(m, "provider", f, o.Provider, cur.Provider, strTag)
	a.scalar(m, "id", f, o.ID, cur.ID, strTag)
	a.scalar(m, "context", f, o.Context, cur.Context, intTag)
	a.scalar(m, "label", f, o.Label, cur.Label, strTag)
	a.scalar(m, "description", f, o.Description, cur.Description, strTag)
	a.settings(m, f, o.Settings, cur.Settings, path)
}

func (a *applier) defaults(old, cur Defaults) {
	if reflect.DeepEqual(old, cur) {
		return
	}
	m := a.mapping(a.root, "defaults", config.Sections, 0)
	f := config.DefaultsFields
	a.scalar(m, "effort", f, old.Effort, cur.Effort, strTag)
	a.settings(m, f, old.Settings, cur.Settings, "defaults")
	a.env(m, f, old.Env, cur.Env)
	a.list(m, "args", f, old.Args, cur.Args)
	if len(m.Content) == 0 {
		a.drop(a.root, find(a.root, "defaults"))
	}
}

func (a *applier) profile(m *yaml.Node, old *Profile, cur Profile, path string) {
	var o Profile
	if old != nil {
		o = *old
	}
	f := config.ProfileFields
	a.scalar(m, "model", f, o.Model, cur.Model, strTag)
	a.scalar(m, "subscription", f, o.Subscription, cur.Subscription, boolTag)
	a.list(m, "picker", f, o.Picker, cur.Picker)
	a.aliases(m, o.Aliases, cur.Aliases)
	a.scalar(m, "subagents", f, o.Subagents, cur.Subagents, strTag)
	a.scalar(m, "effort", f, o.Effort, cur.Effort, strTag)
	a.settings(m, f, o.Settings, cur.Settings, path)
	a.env(m, f, o.Env, cur.Env)
	a.list(m, "args", f, o.Args, cur.Args)
}

// scalar sets field of mapping m from old to cur. order gives the place of
// a new field; tag gives the tag of a value.
func (a *applier) scalar(m *yaml.Node, field string, order []string, old, cur string, tag func(string) string) {
	if old == cur {
		return
	}
	i := find(m, field)
	switch {
	case cur == "":
		a.drop(m, i)
	case i >= 0 && m.Content[i+1].Kind == yaml.ScalarNode:
		// In place: the value keeps its comments.
		v := m.Content[i+1]
		a.unshare(v)
		v.Value, v.Tag = cur, tag(cur)
		if v.Style&(yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) != 0 {
			v.Style = 0
		}
	default:
		// No field yet, or an alias, which gets a value of its own.
		a.set(m, order, field, i, scalarNode(cur, tag(cur)))
	}
}

func strTag(string) string { return "!!str" }

// intTag keeps a value that is not a number a string, which config.Parse
// then reports, instead of writing text YAML would read as something else.
// A leading zero makes YAML read digits as an octal number, so 0400000 is
// text too: only 0 itself may start with a zero.
func intTag(v string) string {
	if v != "" && strings.Trim(v, "0123456789") == "" && (v == "0" || v[0] != '0') {
		return "!!int"
	}
	return "!!str"
}

func boolTag(v string) string {
	if v == "true" || v == "false" {
		return "!!bool"
	}
	return "!!str"
}

// list sets the list field of m from old to cur. A rebuilt list keeps the
// style and comments of the old one; a new one is written as [a, b].
func (a *applier) list(m *yaml.Node, field string, order []string, old, cur []string) {
	if slices.Equal(old, cur) {
		return
	}
	i := find(m, field)
	if len(cur) == 0 {
		a.drop(m, i)
		return
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	if i >= 0 && m.Content[i+1].Kind == yaml.SequenceNode {
		keepLook(seq, m.Content[i+1])
	}
	for _, s := range cur {
		seq.Content = append(seq.Content, scalarNode(s, "!!str"))
	}
	a.set(m, order, field, i, seq)
}

// env sets the env field of m from old to cur. A variable that keeps its
// name keeps its key node, and its value node when the value is the same.
func (a *applier) env(m *yaml.Node, order []string, old, cur []Pair) {
	if slices.Equal(old, cur) {
		return
	}
	i := find(m, "env")
	if len(cur) == 0 {
		a.drop(m, i)
		return
	}
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	var prev *yaml.Node
	if i >= 0 && m.Content[i+1].Kind == yaml.MappingNode {
		prev = m.Content[i+1]
		keepLook(out, prev)
	}
	for _, p := range cur {
		var k, v *yaml.Node
		if j := find(prev, p[0]); j >= 0 {
			k, v = prev.Content[j], prev.Content[j+1]
		}
		if k == nil {
			k = scalarNode(p[0], "!!str")
		}
		if v == nil || v.Kind != yaml.ScalarNode || v.Value != p[1] || isNull(v) {
			v = scalarNode(p[1], "!!str")
		}
		out.Content = append(out.Content, k, v)
	}
	a.set(m, order, "env", i, out)
}

// aliases sets the aliases of m from old to cur, one alias at a time, so
// that the names the draft does not know stay.
func (a *applier) aliases(m *yaml.Node, old, cur map[string]string) {
	var am *yaml.Node
	for _, name := range config.AliasNames {
		if old[name] == cur[name] {
			continue
		}
		if am == nil {
			am = a.mapping(m, "aliases", config.ProfileFields, yaml.FlowStyle)
		}
		a.scalar(am, name, config.AliasNames, old[name], cur[name], strTag)
	}
	if am != nil && len(am.Content) == 0 {
		a.drop(m, find(m, "aliases"))
	}
}

// key sets the key source of provider m: a mapping with one source, in
// the style of the old one, or {kind: value} when there was none.
func (a *applier) key(m *yaml.Node, old, cur Key) {
	if old == cur {
		return
	}
	i := find(m, "key")
	if cur.Kind == "" {
		a.drop(m, i)
		return
	}
	k := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
	if i >= 0 && m.Content[i+1].Kind == yaml.MappingNode {
		keepLook(k, m.Content[i+1])
	}
	k.Content = []*yaml.Node{scalarNode(cur.Kind, "!!str"), scalarNode(cur.Value, "!!str")}
	a.set(m, config.ProviderFields, "key", i, k)
}

// settings sets the settings of m from the text old to the text cur.
func (a *applier) settings(m *yaml.Node, order []string, old, cur, path string) {
	if old == cur {
		return
	}
	i := find(m, "settings")
	n, err := parseSnippet(cur)
	switch {
	case err != nil:
		a.errorf(path+".settings", "%v", err)
	case n == nil:
		a.drop(m, i)
	default:
		a.set(m, order, "settings", i, n)
	}
}

// parseSnippet returns the value of a settings text, nil for a text with
// no value: empty, only comments, or null. The value stands on its own: an
// anchor in the text would take the place of one of the file for the
// aliases after it, so the anchors go and each alias is a copy.
func parseSnippet(text string) (*yaml.Node, error) {
	doc, err := decode([]byte(text))
	if err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || isNull(doc.Content[0]) {
		return nil, nil
	}
	return expand(doc.Content[0])
}

// mapping returns the mapping of field in m, made a node of m's own. A
// missing field gets an empty mapping of style in its place by order.
func (a *applier) mapping(m *yaml.Node, field string, order []string, style yaml.Style) *yaml.Node {
	i := find(m, field)
	if i < 0 {
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: style}
		insert(m, order, field, n)
		return n
	}
	return a.own(m, i+1)
}

// own makes m.Content[i] a mapping that m alone changes: a copy of what an
// alias points at, an empty mapping in place of null, and no longer the
// target of aliases.
func (a *applier) own(m *yaml.Node, i int) *yaml.Node {
	n := m.Content[i]
	switch {
	case n.Kind == yaml.AliasNode:
		n = clone(deref(n))
		m.Content[i] = n
	case isNull(n):
		a.unshare(n)
		n = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map",
			HeadComment: n.HeadComment, LineComment: n.LineComment, FootComment: n.FootComment}
		m.Content[i] = n
	default:
		a.unshare(n)
	}
	return n
}

// set puts v as the value of field: in place of the value at index i of m,
// or in its place by order when i is -1.
func (a *applier) set(m *yaml.Node, order []string, field string, i int, v *yaml.Node) {
	if i < 0 {
		insert(m, order, field, v)
		return
	}
	a.unshareAll(m.Content[i+1])
	m.Content[i+1] = v
}

// drop removes the pair at index i of m, and gives the aliases of its key
// and value copies; -1 does nothing.
func (a *applier) drop(m *yaml.Node, i int) {
	if i >= 0 {
		a.unshare(m.Content[i])
		a.unshareAll(m.Content[i+1])
		remove(m, i)
	}
}

// unshare gives every alias of n a copy of n's value and takes n's anchor
// away, so that a change of n stays in n.
func (a *applier) unshare(n *yaml.Node) {
	if n.Anchor == "" {
		return
	}
	var walk func(p *yaml.Node)
	walk = func(p *yaml.Node) {
		for i, c := range p.Content {
			if c.Kind == yaml.AliasNode && c.Alias == n {
				p.Content[i] = clone(n)
				continue
			}
			walk(c)
		}
	}
	walk(a.root)
	n.Anchor = ""
}

// unshareAll unshares n and every node under it: n is about to go.
func (a *applier) unshareAll(n *yaml.Node) {
	a.unshare(n)
	if n.Kind != yaml.AliasNode {
		for _, c := range n.Content {
			a.unshareAll(c)
		}
	}
}

// clone copies n without its anchors: the anchors stay with the original,
// which other aliases point at.
func clone(n *yaml.Node) *yaml.Node {
	c := *n
	c.Anchor = ""
	c.Content = nil
	if n.Kind != yaml.AliasNode {
		for _, child := range n.Content {
			c.Content = append(c.Content, clone(child))
		}
	}
	return &c
}

// keepLook gives n the style and comments of prev, which n replaces.
func keepLook(n, prev *yaml.Node) {
	n.Style = prev.Style
	n.HeadComment, n.LineComment, n.FootComment = prev.HeadComment, prev.LineComment, prev.FootComment
}

// insert adds field before the first key of m that comes after it in
// order, or at the end.
func insert(m *yaml.Node, order []string, field string, v *yaml.Node) {
	at := len(m.Content)
	if rank := slices.Index(order, field); rank >= 0 {
		for i := 0; i+1 < len(m.Content); i += 2 {
			if r := slices.Index(order, m.Content[i].Value); r > rank {
				at = i
				break
			}
		}
	}
	m.Content = slices.Insert(m.Content, at, scalarNode(field, "!!str"), v)
}

// find returns the index of key in mapping m, or -1.
func find(m *yaml.Node, key string) int {
	if m == nil {
		return -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i]; k.Kind == yaml.ScalarNode && k.Value == key {
			return i
		}
	}
	return -1
}

// remove deletes the pair at index i of mapping m; -1 does nothing.
func remove(m *yaml.Node, i int) {
	if i >= 0 {
		m.Content = slices.Delete(m.Content, i, i+2)
	}
}

func scalarNode(value, tag string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}
