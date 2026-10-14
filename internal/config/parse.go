package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"
)

var (
	providerName = regexp.MustCompile(`^[a-z0-9_-]+$`)
	entityName   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	envName      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Names taken by graft or by Claude Code.
var (
	reservedProviders = []string{"anthropic", "graft"}
	reservedModels    = []string{"default", "best", "opus", "sonnet", "haiku", "fable", "opusplan", "inherit"}
	reservedProfiles  = []string{"list", "check", "help", "version", "ui"}
)

// builtSettings are settings keys graft builds itself, with what to use instead.
var builtSettings = map[string]string{
	"env":           "use env",
	"modelPicker":   "use picker",
	"modelSettings": "use the settings of the model",
}

// Load reads and validates the config file at path. When the file has
// problems, the error is Errors with every problem found.
func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return Parse(data, abs)
}

// Parse validates the config in data. path is the absolute path the data
// came from: messages name it, and relative paths in the config are relative
// to its directory.
func Parse(data []byte, path string) (*Config, error) {
	p := &parser{file: Tildify(path), dir: filepath.Dir(path)}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc, next yaml.Node
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, Errors{{File: p.file, Msg: err.Error()}}
	}
	// Anything after a second --- would be ignored silently.
	switch err := dec.Decode(&next); {
	case err == nil:
		p.errorf(&next, "", "the config must be one YAML document")
	case !errors.Is(err, io.EOF):
		return nil, Errors{{File: p.file, Msg: err.Error()}}
	}
	var root *yaml.Node
	if len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	cfg := &Config{
		Path:      path,
		Providers: map[string]*Provider{},
		Models:    map[string]*Model{},
		Profiles:  map[string]*Profile{},
	}
	top := p.fields(root, "", Sections...)
	p.providers(cfg, top["providers"])
	p.models(cfg, top["models"])
	cfg.Defaults = p.layer(p.fields(top["defaults"], "defaults", DefaultsFields...), "defaults")
	p.profiles(cfg, top["profiles"])
	if len(p.errs) > 0 {
		return nil, p.errs
	}
	return cfg, nil
}

type parser struct {
	file string // the config file as shown in messages
	dir  string // its directory
	errs Errors

	badProviders, badModels rejected
}

// rejected remembers the names a section declares but rejects. A reference
// to such a name is not an error of its own: the error is already reported.
type rejected struct {
	all   bool // the section is not a mapping
	names map[string]bool
}

func (r *rejected) add(name string) {
	if r.names == nil {
		r.names = map[string]bool{}
	}
	r.names[name] = true
}

func (r *rejected) has(name string) bool { return r.all || r.names[name] }

// notMapping reports whether n is set but is not a mapping.
func notMapping(n *yaml.Node) bool {
	n = deref(n)
	return n != nil && !isNull(n) && n.Kind != yaml.MappingNode
}

func (p *parser) errorf(n *yaml.Node, path, format string, args ...any) {
	line := 0
	if n != nil {
		line = n.Line
	}
	p.errs = append(p.errs, Error{File: p.file, Line: line, Path: path, Msg: fmt.Sprintf(format, args...)})
}

// entry is one key and value of a mapping.
type entry struct {
	key     string
	keyNode *yaml.Node
	value   *yaml.Node
}

// entries returns the pairs of mapping n in file order. A missing or null
// node is an empty mapping; any other node that is not a mapping, and a
// repeated key, are errors.
func (p *parser) entries(n *yaml.Node, path string) []entry {
	n = deref(n)
	if n == nil || isNull(n) {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		p.errorf(n, path, "must be a mapping")
		return nil
	}
	var out []entry
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := deref(n.Content[i]), deref(n.Content[i+1])
		if k.Kind != yaml.ScalarNode {
			p.errorf(k, path, "keys must be strings")
			continue
		}
		if seen[k.Value] {
			p.errorf(k, join(path, k.Value), "duplicate key")
			continue
		}
		seen[k.Value] = true
		out = append(out, entry{key: k.Value, keyNode: k, value: v})
	}
	return out
}

// fields returns the values of mapping n by key; a key outside allowed is
// an error.
func (p *parser) fields(n *yaml.Node, path string, allowed ...string) map[string]*yaml.Node {
	out := map[string]*yaml.Node{}
	for _, e := range p.entries(n, path) {
		if !slices.Contains(allowed, e.key) {
			p.errorf(e.keyNode, join(path, e.key), "unknown field")
			continue
		}
		out[e.key] = e.value
	}
	return out
}

// str returns a scalar as a string; null is empty.
func (p *parser) str(n *yaml.Node, path string) string {
	if n == nil || isNull(n) {
		return ""
	}
	if n.Kind != yaml.ScalarNode {
		p.errorf(n, path, "must be a string")
		return ""
	}
	return n.Value
}

func (p *parser) integer(n *yaml.Node, path string) (int, bool) {
	var v int
	if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!int" || n.Decode(&v) != nil {
		p.errorf(n, path, "must be an integer")
		return 0, false
	}
	return v, true
}

func (p *parser) boolean(n *yaml.Node, path string) (bool, bool) {
	var v bool
	if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!bool" || n.Decode(&v) != nil {
		p.errorf(n, path, "must be true or false")
		return false, false
	}
	return v, true
}

// item is one string of a list.
type item struct {
	value string
	node  *yaml.Node
	path  string
}

// list returns the strings of sequence n; null is an empty list.
func (p *parser) list(n *yaml.Node, path string) []item {
	if n == nil || isNull(n) {
		return nil
	}
	if n.Kind != yaml.SequenceNode {
		p.errorf(n, path, "must be a list")
		return nil
	}
	var out []item
	for i, c := range n.Content {
		c = deref(c)
		ip := fmt.Sprintf("%s[%d]", path, i)
		if c.Kind != yaml.ScalarNode || isNull(c) {
			p.errorf(c, ip, "must be a string")
			continue
		}
		out = append(out, item{value: c.Value, node: c, path: ip})
	}
	return out
}

// object decodes mapping n into values that encoding/json can marshal.
func (p *parser) object(n *yaml.Node, path string) map[string]any {
	if n == nil || isNull(n) {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		p.errorf(n, path, "must be a mapping")
		return nil
	}
	if !p.jsonReady(n, path, map[*yaml.Node]bool{}) {
		return nil
	}
	// With string keys only, yaml.v3 decodes every mapping as map[string]any.
	var v map[string]any
	if err := n.Decode(&v); err != nil {
		p.errorf(n, path, "%v", err)
		return nil
	}
	return v
}

// jsonReady reports, each at its line, what cannot go from YAML into JSON:
// merge keys, which yaml.v3 would apply past the checks on the keys; keys
// that are not strings or that repeat; infinities, NaN and timestamps. It
// returns false when it found any.
func (p *parser) jsonReady(n *yaml.Node, path string, seen map[*yaml.Node]bool) bool {
	n = deref(n)
	if n == nil || seen[n] {
		return true
	}
	seen[n] = true
	ok := true
	switch n.Kind {
	case yaml.MappingNode:
		keys := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := deref(n.Content[i])
			switch {
			case k.ShortTag() == "!!merge":
				p.errorf(k, path, "merge keys (<<) are not supported")
				ok = false
			case k.Kind != yaml.ScalarNode || k.ShortTag() != "!!str":
				p.errorf(k, path, "keys must be strings")
				ok = false
			case keys[k.Value]:
				p.errorf(k, join(path, k.Value), "duplicate key")
				ok = false
			default:
				keys[k.Value] = true
				if !p.jsonReady(n.Content[i+1], join(path, k.Value), seen) {
					ok = false
				}
			}
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			if !p.jsonReady(c, fmt.Sprintf("%s[%d]", path, i), seen) {
				ok = false
			}
		}
	case yaml.ScalarNode:
		var f float64
		switch {
		case n.ShortTag() == "!!float" && n.Decode(&f) == nil && (math.IsInf(f, 0) || math.IsNaN(f)):
			p.errorf(n, path, "%s is not a JSON number", n.Value)
			ok = false
		case n.ShortTag() == "!!timestamp":
			// yaml.v3 makes it a time, which JSON would show in another form.
			p.errorf(n, path, "%s is a YAML timestamp, which JSON lacks: quote it", n.Value)
			ok = false
		}
	}
	return ok
}

func (p *parser) providers(cfg *Config, n *yaml.Node) {
	p.badProviders.all = notMapping(n)
	for _, e := range p.entries(n, "providers") {
		path := "providers." + e.key
		switch {
		case !providerName.MatchString(e.key):
			p.errorf(e.keyNode, path, "a provider name may contain lowercase letters, digits, '-' and '_'")
			p.badProviders.add(e.key)
			continue
		case slices.Contains(reservedProviders, e.key):
			p.errorf(e.keyNode, path, "%q is reserved", e.key)
			p.badProviders.add(e.key)
			continue
		}
		before := len(p.errs)
		f := p.fields(e.value, path, ProviderFields...)
		prov := &Provider{Name: e.key}
		if un := f["url"]; un == nil {
			p.errorf(e.keyNode, path+".url", "required")
		} else {
			prov.URL = p.str(un, path+".url")
			if msg := checkURL(prov.URL); msg != "" {
				p.errorf(un, path+".url", "%s", msg)
			}
		}
		if kn := f["key"]; kn == nil {
			p.errorf(e.keyNode, path+".key", "required")
		} else {
			prov.Key = p.keySource(kn, path+".key")
		}
		if len(p.errs) > before {
			p.badProviders.add(e.key)
			continue
		}
		cfg.Providers[e.key] = prov
		cfg.ProviderOrder = append(cfg.ProviderOrder, e.key)
	}
}

// checkURL says what is wrong with a provider URL, or returns "". Claude
// Code appends the API path to the URL, so it cannot end in a query or a
// fragment, and the key comes from key, not from the URL.
func checkURL(s string) string {
	u, err := url.Parse(s)
	switch {
	case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "":
		return "must be an absolute http or https URL"
	case u.User != nil:
		return "must not contain a user name or password: the key comes from key"
	case strings.ContainsAny(s, "?#"):
		return "must not have a query or fragment: Claude Code appends /v1/messages to it"
	}
	return ""
}

func (p *parser) keySource(n *yaml.Node, path string) KeySource {
	if n.Kind == yaml.ScalarNode && !isNull(n) {
		p.errorf(n, path, "say where the key comes from: env, file or command; the key itself does not go into the config")
		return KeySource{}
	}
	f := p.fields(n, path, KeyFields...)
	var src KeySource
	if v := f["env"]; v != nil {
		if src.Env = p.str(v, path+".env"); !envName.MatchString(src.Env) {
			p.errorf(v, path+".env", "not a valid variable name")
		}
	}
	if v := f["file"]; v != nil {
		if s := p.str(v, path+".file"); s == "" {
			p.errorf(v, path+".file", "must not be empty")
		} else if abs, err := p.resolvePath(s); err != nil {
			p.errorf(v, path+".file", "%v", err)
		} else {
			src.File = abs
		}
	}
	if v := f["command"]; v != nil {
		if src.Command = p.str(v, path+".command"); strings.TrimSpace(src.Command) == "" {
			p.errorf(v, path+".command", "must not be empty")
		}
	}
	if m := deref(n); (m.Kind == yaml.MappingNode || isNull(m)) && len(f) != 1 {
		p.errorf(n, path, "set exactly one of env, file, command")
	}
	return src
}

// resolvePath expands a leading ~ and makes a relative path relative to the
// directory of the config file.
func (p *parser) resolvePath(s string) (string, error) {
	if s == "~" || strings.HasPrefix(s, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		s = filepath.Join(home, s[1:])
	}
	if !filepath.IsAbs(s) {
		s = filepath.Join(p.dir, s)
	}
	return s, nil
}

func (p *parser) models(cfg *Config, n *yaml.Node) {
	p.badModels.all = notMapping(n)
	for _, e := range p.entries(n, "models") {
		path := "models." + e.key
		lower := strings.ToLower(e.key)
		switch {
		case !entityName.MatchString(e.key):
			p.errorf(e.keyNode, path, "a model name may contain letters, digits, '.', '_' and '-', and starts with a letter or digit")
			p.badModels.add(e.key)
			continue
		case strings.HasPrefix(lower, "claude"):
			p.errorf(e.keyNode, path, "a model name must not start with \"claude\": such names belong to the subscription")
			p.badModels.add(e.key)
			continue
		case slices.Contains(reservedModels, lower):
			p.errorf(e.keyNode, path, "%q is a Claude Code alias", e.key)
			p.badModels.add(e.key)
			continue
		}
		before := len(p.errs)
		f := p.fields(e.value, path, ModelFields...)
		m := &Model{Name: e.key, ID: e.key, Label: e.key}
		if pn := f["provider"]; pn == nil {
			p.errorf(e.keyNode, path+".provider", "required")
		} else if m.Provider = p.str(pn, path+".provider"); pn.Kind == yaml.ScalarNode &&
			cfg.Providers[m.Provider] == nil && !p.badProviders.has(m.Provider) {
			p.errorf(pn, path+".provider", "no provider %q", m.Provider)
		}
		if in := f["id"]; in != nil {
			if m.ID = p.str(in, path+".id"); m.ID == "" || strings.ContainsFunc(m.ID, unicode.IsSpace) {
				p.errorf(in, path+".id", "must be non-empty and contain no white space")
			}
		}
		if cn := f["context"]; cn == nil {
			p.errorf(e.keyNode, path+".context", "required")
		} else if v, ok := p.integer(cn, path+".context"); ok {
			if v < MinContext {
				p.errorf(cn, path+".context", "must be at least %d", MinContext)
			}
			m.Context = v
		}
		if s := p.str(f["label"], path+".label"); s != "" {
			m.Label = s
		}
		m.Description = "via " + m.Provider
		if s := p.str(f["description"], path+".description"); s != "" {
			m.Description = s
		}
		m.Settings = p.object(f["settings"], path+".settings")
		if len(p.errs) > before {
			p.badModels.add(e.key)
			continue
		}
		cfg.Models[e.key] = m
	}
}

// layer reads the fields that defaults and profiles share.
func (p *parser) layer(f map[string]*yaml.Node, path string) Layer {
	var l Layer
	if n := f["effort"]; n != nil {
		if l.Effort = p.str(n, path+".effort"); !slices.Contains(Efforts, l.Effort) {
			p.errorf(n, path+".effort", "must be one of %s", strings.Join(Efforts, ", "))
		}
	}
	if n := f["settings"]; n != nil {
		sp := path + ".settings"
		if l.Settings = p.object(n, sp); l.Settings != nil {
			for _, e := range p.entries(n, sp) {
				if hint, ok := builtSettings[e.key]; ok {
					p.errorf(e.keyNode, sp+"."+e.key, "graft builds %s itself: %s", e.key, hint)
				}
			}
		}
	}
	if n := f["env"]; n != nil {
		l.Env = p.env(n, path+".env")
	}
	for _, it := range p.list(f["args"], path+".args") {
		if it.value == "--settings" || strings.HasPrefix(it.value, "--settings=") {
			p.errorf(it.node, it.path, "--settings is managed by graft: put Claude Code settings into settings")
			continue
		}
		l.Args = append(l.Args, it.value)
	}
	return l
}

func (p *parser) env(n *yaml.Node, path string) map[string]string {
	out := map[string]string{}
	for _, e := range p.entries(n, path) {
		ep := path + "." + e.key
		switch {
		case !envName.MatchString(e.key):
			p.errorf(e.keyNode, ep, "not a valid variable name")
		case slices.Contains(OwnedEnv, e.key):
			p.errorf(e.keyNode, ep, "graft manages this variable itself")
		case e.value.Kind != yaml.ScalarNode:
			p.errorf(e.value, ep, "must be a string")
		case isNull(e.value): // ~ or null would reach claude as that text
			p.errorf(e.value, ep, `must be a string: write "" for an empty value`)
		default:
			out[e.key] = e.value.Value
		}
	}
	return out
}

func (p *parser) profiles(cfg *Config, n *yaml.Node) {
	es := p.entries(n, "profiles")
	if len(es) == 0 {
		if m := deref(n); m == nil || isNull(m) || m.Kind == yaml.MappingNode {
			p.errorf(n, "profiles", "at least one profile is required")
		}
		return
	}
	for _, e := range es {
		path := "profiles." + e.key
		switch {
		case !entityName.MatchString(e.key):
			p.errorf(e.keyNode, path, "a profile name may contain letters, digits, '.', '_' and '-', and starts with a letter or digit")
			continue
		case slices.Contains(reservedProfiles, e.key):
			p.errorf(e.keyNode, path, "%q is a graft command", e.key)
			continue
		}
		if prof := p.profile(cfg, e, path); prof != nil {
			cfg.Profiles[e.key] = prof
			cfg.ProfileOrder = append(cfg.ProfileOrder, e.key)
		}
	}
}

// profile parses one profile; it returns nil when the profile has errors.
func (p *parser) profile(cfg *Config, e entry, path string) *Profile {
	before := len(p.errs)
	f := p.fields(e.value, path, ProfileFields...)
	prof := &Profile{Name: e.key, Layer: p.layer(f, path)}

	// Claude models the profile names: they need the subscription.
	type use struct {
		node  *yaml.Node
		path  string
		value string
	}
	var claude []use
	ref := func(n *yaml.Node, rp string, aliasOK bool) (Ref, bool) {
		s := p.str(n, rp)
		switch {
		case n.Kind != yaml.ScalarNode:
			// p.str has reported that it is not a string.
		case s == "":
			p.errorf(n, rp, "must name a model")
		case cfg.Models[s] != nil:
			return Ref{Model: s}, true
		case aliasOK && slices.Contains(AliasNames, s), strings.HasPrefix(strings.ToLower(s), "claude-"):
			claude = append(claude, use{n, rp, s})
			return Ref{Claude: s}, true
		case p.badModels.has(s):
			// The model has an error of its own.
		default:
			p.errorf(n, rp, "unknown model %q", s)
		}
		return Ref{}, false
	}

	if mn := f["model"]; mn == nil {
		p.errorf(e.keyNode, path+".model", "required")
	} else {
		prof.Model, _ = ref(mn, path+".model", true)
	}
	prof.Subscription = prof.Model.Claude != ""
	if sn := f["subscription"]; sn != nil {
		if v, ok := p.boolean(sn, path+".subscription"); ok {
			prof.Subscription = v
		}
	}
	for _, it := range p.list(f["picker"], path+".picker") {
		switch {
		case cfg.Models[it.value] != nil:
			prof.Picker = append(prof.Picker, it.value)
		case !p.badModels.has(it.value): // a rejected model has an error of its own
			p.errorf(it.node, it.path, "unknown model %q: picker lists the models of this config", it.value)
		}
	}
	if an := f["aliases"]; an != nil {
		prof.Aliases = map[string]Ref{}
		for _, a := range p.entries(an, path+".aliases") {
			ap := path + ".aliases." + a.key
			if !slices.Contains(AliasNames, a.key) {
				p.errorf(a.keyNode, ap, "unknown alias: use %s", strings.Join(AliasNames, ", "))
				continue
			}
			if r, ok := ref(a.value, ap, false); ok {
				prof.Aliases[a.key] = r
			}
		}
	}
	if sn := f["subagents"]; sn != nil {
		if r, ok := ref(sn, path+".subagents", true); ok {
			prof.Subagents = &r
		}
	}
	if !prof.Subscription {
		for _, c := range claude {
			p.errorf(c.node, c.path, "%q is a Claude model: it needs subscription: true", c.value)
		}
	}
	if len(p.errs) > before {
		return nil
	}
	if cfg.ModeOf(prof) == Direct {
		// Claude Code sees the ids in the direct mode, so they must differ.
		ids := map[string]string{}
		for _, m := range cfg.ExternalModels(prof) {
			if other, ok := ids[m.ID]; ok {
				p.errorf(e.keyNode, path, "models %q and %q share the id %q, and a direct profile shows Claude Code the ids", other, m.Name, m.ID)
			}
			ids[m.ID] = m.Name
		}
	}
	if len(p.errs) > before {
		return nil
	}
	return prof
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
