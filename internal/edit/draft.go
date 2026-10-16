// Package edit turns a config file into the draft that graft ui edits, and
// applies a changed draft back to the file. What the draft does not change
// keeps its comments, style and anchors, and fields the draft does not know
// stay as they are. It does no I/O.
package edit

import (
	"encoding/json"
	"fmt"
	"reflect"

	"go.yaml.in/yaml/v3"

	"github.com/zinin/graft/internal/config"
)

// Draft is a config as its file writes it: values are strings as written,
// and "" is a field the file does not set.
type Draft struct {
	Providers []Provider `json:"providers"`
	Models    []Model    `json:"models"`
	Defaults  Defaults   `json:"defaults"`
	Profiles  []Profile  `json:"profiles"`
}

// Provider is an entry of providers. Was is its name in the file, "" for an
// entry the file does not have yet; so are the Was of models and profiles.
type Provider struct {
	Was  string `json:"was"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Key  Key    `json:"key"`
}

// Key is a key source. Kind is env, file, command, or "" when the file
// names none; Value is the variable, path or command as written.
type Key struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Model is an entry of models. Settings is YAML text, and Context is the
// value as YAML writes it, quotes and tag included (see spelling): "400000"
// in quotes is a string, which config.Parse rejects.
type Model struct {
	Was         string `json:"was"`
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	ID          string `json:"id"`
	Context     string `json:"context"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Settings    string `json:"settings"`
}

// Defaults is the defaults section.
type Defaults struct {
	Effort   string   `json:"effort"`
	Settings string   `json:"settings"`
	Env      []Pair   `json:"env"`
	Args     []string `json:"args"`
}

// Profile is an entry of profiles. Subscription is the value as YAML writes
// it, like the Context of a model. Aliases holds every name of
// config.AliasNames, "" for an alias the file does not set.
type Profile struct {
	Was          string            `json:"was"`
	Name         string            `json:"name"`
	Model        string            `json:"model"`
	Subscription string            `json:"subscription"`
	Picker       []string          `json:"picker"`
	Aliases      map[string]string `json:"aliases"`
	Subagents    string            `json:"subagents"`
	Effort       string            `json:"effort"`
	Settings     string            `json:"settings"`
	Env          []Pair            `json:"env"`
	Args         []string          `json:"args"`
}

// Pair is a variable of env, [name, value] in JSON. A value of "" is the
// empty string. Null marks a variable the file gives no value, such as FOO:
// or FOO: ~, which config.Parse rejects: its value is null in JSON, and
// Value is "".
type Pair struct {
	Name, Value string
	Null        bool
}

// MarshalJSON writes p as [name, value], with null for no value.
func (p Pair) MarshalJSON() ([]byte, error) {
	var value any = p.Value
	if p.Null {
		value = nil
	}
	return json.Marshal([2]any{p.Name, value})
}

// UnmarshalJSON reads p from [name, value]: a string, and a string or null
// for no value. Any other shape is an error, so that a request cannot stand
// for a variable it does not spell out, such as ["FOO"] or [].
func (p *Pair) UnmarshalJSON(data []byte) error {
	var v []json.RawMessage
	var name, value string
	// encoding/json takes null for a string with no error, so null is looked
	// for first: as the name it is an error, as the value no value.
	ok := json.Unmarshal(data, &v) == nil && len(v) == 2 &&
		string(v[0]) != "null" && json.Unmarshal(v[0], &name) == nil &&
		(string(v[1]) == "null" || json.Unmarshal(v[1], &value) == nil)
	if !ok {
		return fmt.Errorf("a variable of env is [name, value], with a string or null for the value, not %s", data)
	}
	*p = Pair{Name: name, Value: value, Null: string(v[1]) == "null"}
	return nil
}

// Normalize fills in what JSON may leave out: empty lists for null, and
// every alias name.
func (d *Draft) Normalize() {
	d.Providers = orEmpty(d.Providers)
	d.Models = orEmpty(d.Models)
	// A copy: the loop below changes the profiles, and a Draft passed by
	// value shares them with its caller.
	d.Profiles = append([]Profile{}, d.Profiles...)
	d.Defaults.Env = orEmpty(d.Defaults.Env)
	d.Defaults.Args = orEmpty(d.Defaults.Args)
	for i := range d.Profiles {
		p := &d.Profiles[i]
		p.Picker = orEmpty(p.Picker)
		p.Env = orEmpty(p.Env)
		p.Args = orEmpty(p.Args)
		aliases := map[string]string{}
		for _, name := range config.AliasNames {
			aliases[name] = p.Aliases[name]
		}
		p.Aliases = aliases
	}
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Same reports whether a and b hold the same config: Was does not count,
// and settings, context and subscription count by value, not by text.
func Same(a, b Draft) bool {
	return reflect.DeepEqual(a.canonical(), b.canonical())
}

// canonical returns a copy of d without Was and with settings, context and
// subscription in one form.
func (d Draft) canonical() Draft {
	d.Normalize()
	out := Draft{Defaults: d.Defaults}
	out.Defaults.Settings = canonicalYAML(d.Defaults.Settings)
	for _, p := range d.Providers {
		p.Was = ""
		out.Providers = append(out.Providers, p)
	}
	for _, m := range d.Models {
		m.Was, m.Settings, m.Context = "", canonicalYAML(m.Settings), canonicalSpelling(m.Context, intTag)
		out.Models = append(out.Models, m)
	}
	for _, p := range d.Profiles {
		p.Was, p.Settings, p.Subscription = "", canonicalYAML(p.Settings), canonicalSpelling(p.Subscription, boolTag)
		out.Profiles = append(out.Profiles, p)
	}
	out.Normalize()
	return out
}

// canonicalSpelling returns the spelling that Read gives the value Apply
// writes for text, the spelling of a value whose YAML type counts: so " 400000 "
// counts as the 400000 that the file reads back. The value is written inside
// [ ] and read back. There YAML puts quotes around more values than in a
// block, such as 1,000,000, so the spelling is the same whether the value
// goes into a block or into { }; and it is what yaml.v3 reads, which is not
// always what it was told to write: it writes the string << without quotes,
// and reads that back as a merge key.
func canonicalSpelling(text string, tag func(string) string) string {
	if text == "" {
		return ""
	}
	if n := reread(parseSpelling(text, tag)); n != nil {
		return spelling(n)
	}
	return text // not reached: parseSpelling gives a value that reads back
}

// canonicalYAML returns the value of YAML text as JSON, or the text itself
// when it is not YAML that JSON can hold.
func canonicalYAML(text string) string {
	var v any
	if yaml.Unmarshal([]byte(text), &v) != nil {
		return text
	}
	data, err := json.Marshal(v)
	if err != nil {
		return text
	}
	return string(data)
}
