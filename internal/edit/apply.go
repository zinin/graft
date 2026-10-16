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
	a.scalar(m, "url", config.ProviderFields, o.URL, cur.URL)
	a.key(m, o.Key, cur.Key)
}

func (a *applier) model(m *yaml.Node, old *Model, cur Model, path string) {
	var o Model
	if old != nil {
		o = *old
	}
	f := config.ModelFields
	a.scalar(m, "provider", f, o.Provider, cur.Provider)
	a.scalar(m, "id", f, o.ID, cur.ID)
	a.typed(m, "context", f, o.Context, cur.Context, intTag)
	a.scalar(m, "label", f, o.Label, cur.Label)
	a.scalar(m, "description", f, o.Description, cur.Description)
	a.settings(m, f, o.Settings, cur.Settings, path)
}

func (a *applier) defaults(old, cur Defaults) {
	if reflect.DeepEqual(old, cur) {
		return
	}
	m := a.mapping(a.root, "defaults", config.Sections, 0)
	f := config.DefaultsFields
	a.scalar(m, "effort", f, old.Effort, cur.Effort)
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
	a.scalar(m, "model", f, o.Model, cur.Model)
	a.typed(m, "subscription", f, o.Subscription, cur.Subscription, boolTag)
	a.list(m, "picker", f, o.Picker, cur.Picker)
	a.aliases(m, o.Aliases, cur.Aliases)
	a.scalar(m, "subagents", f, o.Subagents, cur.Subagents)
	a.scalar(m, "effort", f, o.Effort, cur.Effort)
	a.settings(m, f, o.Settings, cur.Settings, path)
	a.env(m, f, o.Env, cur.Env)
	a.list(m, "args", f, o.Args, cur.Args)
}

// scalar sets the string field of mapping m from old to cur. order gives
// the place of a new field.
func (a *applier) scalar(m *yaml.Node, field string, order []string, old, cur string) {
	if old == cur {
		return
	}
	i := find(m, field)
	switch {
	case cur == "":
		a.drop(m, i)
	case i >= 0 && m.Content[i+1].Kind == yaml.ScalarNode:
		// In place: the value keeps its comments.
		a.rewrite(m.Content[i+1], cur, "!!str")
	default:
		// No field yet, or an alias, which gets a value of its own.
		a.set(m, order, field, i, scalarNode(cur, "!!str"))
	}
}

// typed sets field of mapping m, a value whose YAML type counts, from the
// spelling old to the spelling cur (see parseSpelling, which takes tag). In
// place, the value keeps its comments, and its aliases get a copy of the old
// value, but it takes the tag and the quotes of cur, not those it had:
// "400000" changed to 400000 becomes a number.
func (a *applier) typed(m *yaml.Node, field string, order []string, old, cur string, tag func(string) string) {
	if old == cur {
		return
	}
	i := find(m, field)
	switch {
	case cur == "":
		a.drop(m, i)
	case i >= 0 && m.Content[i+1].Kind == yaml.ScalarNode:
		n, v := parseSpelling(cur, tag), m.Content[i+1]
		a.unshare(v)
		v.Tag, v.Value, v.Style = n.Tag, n.Value, n.Style
	default:
		// No field yet, or an alias, which gets a value of its own.
		a.set(m, order, field, i, parseSpelling(cur, tag))
	}
}

// parseSpelling returns the value that Apply writes for text, the spelling
// of a value whose YAML type counts (see spelling): text is YAML. A plain
// value, with no quotes and no tag, takes its tag from tag. A value in
// quotes or with a tag keeps them, so "400000" is a string. Any other text
// is a string of the text itself, for config.Parse to report: a text that
// does not parse or holds two documents, a list or a mapping, which would
// keep the forms closed, a null with a tag, which would read back as no
// value, and a value that yaml.v3 cannot read back once it has written it,
// such as one with the tag !%21, which it writes as !!. The value goes on
// one line, as the field shows it (see oneLine), without the anchor and the
// comments of the text. On one line, it also stays clear of a block scalar,
// which yaml.v3 writes so that it cannot read it back when the value starts
// with a tab.
func parseSpelling(text string, tag func(string) string) *yaml.Node {
	var n *yaml.Node
	doc, err := decode([]byte(text))
	if err == nil && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.ScalarNode {
		const marks = yaml.TaggedStyle | yaml.SingleQuotedStyle | yaml.DoubleQuotedStyle | yaml.LiteralStyle | yaml.FoldedStyle
		switch r := doc.Content[0]; {
		case r.Style&marks == 0:
			n = oneLine(scalarNode(r.Value, tag(r.Value)))
		case !isNull(r):
			n = oneLine(r)
		}
	}
	if n == nil || reread(n) == nil {
		n = oneLine(scalarNode(text, "!!str"))
	}
	return n
}

// reread returns scalar n as YAML reads it back once it is written inside
// [ ], or nil when it cannot be read back.
func reread(n *yaml.Node) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle, Content: []*yaml.Node{n}}
	data, err := encode(seq)
	if err != nil {
		return nil
	}
	doc, err := decode(data)
	if err != nil || len(doc.Content) != 1 || len(doc.Content[0].Content) != 1 || doc.Content[0].Content[0].Kind != yaml.ScalarNode {
		return nil
	}
	return doc.Content[0].Content[0]
}

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
// style and comments of the old one, and each item that stays keeps its
// node: its comments, its anchor, or the alias it is. A new list is written
// as [a, b].
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
	var prev *yaml.Node
	if i >= 0 && m.Content[i+1].Kind == yaml.SequenceNode {
		prev = m.Content[i+1]
		keepLook(seq, prev)
	}
	for _, s := range cur {
		item := scalarNode(s, "!!str")
		if prev != nil {
			// The first old item of this text that no new item has taken.
			j := slices.IndexFunc(prev.Content, func(c *yaml.Node) bool {
				return holds(c, s) && !slices.Contains(seq.Content, c)
			})
			if j >= 0 {
				item = prev.Content[j]
			}
		}
		seq.Content = append(seq.Content, item)
	}
	if prev == nil {
		a.set(m, order, field, i, seq)
		return
	}
	a.rebuild(m, i, seq)
}

// env sets the env field of m from old to cur. A variable that keeps its
// name keeps its key node, and its value node: as it is when the value is
// the same, an alias too, and changed in place when it is not, so that the
// value keeps its comments. An alias whose value changes gets a value of
// its own, and so does one that the new order puts before its anchor.
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
	var changed []int // the indexes in cur of the values to change in place
	for n, p := range cur {
		var k, v *yaml.Node
		if j := find(prev, p[0]); j >= 0 {
			k, v = prev.Content[j], prev.Content[j+1]
		}
		switch {
		case k == nil:
			k, v = scalarNode(p[0], "!!str"), scalarNode(p[1], "!!str")
		case holds(v, p[1]):
			// The same value: the node stays, an alias too.
		case v.Kind == yaml.ScalarNode:
			changed = append(changed, n)
		default:
			// An alias whose value changed, or not a scalar.
			v = scalarNode(p[1], "!!str")
		}
		out.Content = append(out.Content, k, v)
	}
	if prev == nil {
		a.set(m, order, "env", i, out)
		return
	}
	a.rebuild(m, i, out)
	// Only now, with out in the tree, so that the aliases in out of a value
	// that changes get a copy of the old value too.
	for _, n := range changed {
		a.rewrite(out.Content[2*n+1], cur[n][1], "!!str")
	}
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
		a.scalar(am, name, config.AliasNames, old[name], cur[name])
	}
	if am != nil && len(am.Content) == 0 {
		a.drop(m, find(m, "aliases"))
	}
}

// key sets the key source of provider m. Of the sources graft knows, only
// the one of cur stays in the mapping, and the fields the draft does not
// know stay where they are, for config.Parse to report. The source of cur
// keeps its nodes when the mapping has it, and its value changes in place,
// so that its comments stay; a new one takes the place of the first old
// source, or goes at the end. When there is no mapping, as when the key
// itself is written into the config, the key becomes {kind: value}.
func (a *applier) key(m *yaml.Node, old, cur Key) {
	if old == cur {
		return
	}
	i := find(m, "key")
	if i < 0 || deref(m.Content[i+1]).Kind != yaml.MappingNode {
		if cur.Kind == "" {
			a.drop(m, i)
			return
		}
		k := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
		k.Content = []*yaml.Node{scalarNode(cur.Kind, "!!str"), scalarNode(cur.Value, "!!str")}
		a.set(m, config.ProviderFields, "key", i, k)
		return
	}
	// An alias gets a mapping of its own, and the aliases of the mapping get
	// a copy of the old one.
	km := a.own(m, i+1)
	switch j := find(km, cur.Kind); {
	case cur.Kind == "":
		// No source: the old ones go, and the key too if nothing is left.
	case j < 0:
		at := len(km.Content) // the place of the first old source, or the end
		for n := 0; n < len(km.Content); n += 2 {
			if slices.Contains(config.KeyFields, km.Content[n].Value) {
				at = n
				break
			}
		}
		km.Content = slices.Insert(km.Content, at, scalarNode(cur.Kind, "!!str"), scalarNode(cur.Value, "!!str"))
	case km.Content[j+1].Kind == yaml.ScalarNode:
		a.rewrite(km.Content[j+1], cur.Value, "!!str")
	default:
		// An alias, which gets a value of its own.
		a.set(km, config.KeyFields, cur.Kind, j, scalarNode(cur.Value, "!!str"))
	}
	// The other sources go.
	for n := 0; n < len(km.Content); {
		if k := km.Content[n].Value; k != cur.Kind && slices.Contains(config.KeyFields, k) {
			a.drop(km, n)
			continue
		}
		n += 2
	}
	if len(km.Content) == 0 {
		a.drop(m, i)
	}
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
// alias points at, an empty mapping in place of null (also when the alias
// points at null), and no longer the target of aliases.
func (a *applier) own(m *yaml.Node, i int) *yaml.Node {
	n := m.Content[i]
	if n.Kind == yaml.AliasNode {
		// The copy is null when the alias points at null.
		n = clone(deref(n))
		m.Content[i] = n
	}
	a.unshare(n)
	if isNull(n) {
		n = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map",
			HeadComment: n.HeadComment, LineComment: n.LineComment, FootComment: n.FootComment}
		m.Content[i] = n
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

// rebuild puts n, a container built from the value at index i of m, in its
// place. n may hold nodes of the old container: those stay as they are, with
// their anchors. The old container and the nodes n does not hold leave the
// tree, so their aliases get copies. An alias that n puts before the node it
// points at becomes a copy too, since YAML reads an alias only after its
// anchor. n holds scalars and aliases, as list and env build it.
func (a *applier) rebuild(m *yaml.Node, i int, n *yaml.Node) {
	prev := m.Content[i+1]
	a.unshare(prev)
	// n goes into the tree first, so that its aliases of the nodes that
	// leave get copies too.
	m.Content[i+1] = n
	for _, c := range prev.Content {
		if !slices.Contains(n.Content, c) {
			a.unshareAll(c)
		}
	}
	// The anchor that the new order can put after its alias is on one of the
	// nodes n holds, which are scalars and aliases. The copy keeps the
	// comments of the alias, which describe its own place, and the anchor
	// stays for the aliases after it.
	for j, c := range n.Content {
		if c.Kind == yaml.AliasNode && slices.Contains(n.Content[j+1:], c.Alias) {
			cp := clone(c.Alias)
			cp.HeadComment, cp.LineComment, cp.FootComment = c.HeadComment, c.LineComment, c.FootComment
			n.Content[j] = cp
		}
	}
}

// rewrite gives scalar v the value and tag in place, so that v keeps its
// comments; the aliases of v get a copy of the old value first.
func (a *applier) rewrite(v *yaml.Node, value, tag string) {
	a.unshare(v)
	v.Value, v.Tag = value, tag
	if v.Style&(yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) != 0 {
		v.Style = 0
	}
	// Quotes would make a value of another type a string.
	if v.Tag != "!!str" {
		v.Style &^= yaml.SingleQuotedStyle | yaml.DoubleQuotedStyle
	}
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

// holds reports whether n, with aliases followed, is a scalar other than
// null with the text s.
func holds(n *yaml.Node, s string) bool {
	v := deref(n)
	return v.Kind == yaml.ScalarNode && !isNull(v) && v.Value == s
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
