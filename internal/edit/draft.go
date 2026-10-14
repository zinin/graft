// Package edit turns a config file into the draft that graft ui edits, and
// applies a changed draft back to the file. What the draft does not change
// keeps its comments, style and anchors, and fields the draft does not know
// stay as they are. It does no I/O.
package edit

import (
	"encoding/json"
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

// Model is an entry of models. Settings is YAML text.
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

// Profile is an entry of profiles. Aliases holds every name of
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
// empty string.
type Pair [2]string

// Normalize fills in what JSON may leave out: empty lists for null, and
// every alias name.
func (d *Draft) Normalize() {
	d.Providers = orEmpty(d.Providers)
	d.Models = orEmpty(d.Models)
	d.Profiles = orEmpty(d.Profiles)
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
// and settings count by value, not by text.
func Same(a, b Draft) bool {
	return reflect.DeepEqual(a.canonical(), b.canonical())
}

// canonical returns a copy of d without Was and with settings in one form.
func (d Draft) canonical() Draft {
	d.Normalize()
	out := Draft{Defaults: d.Defaults}
	out.Defaults.Settings = canonicalYAML(d.Defaults.Settings)
	for _, p := range d.Providers {
		p.Was = ""
		out.Providers = append(out.Providers, p)
	}
	for _, m := range d.Models {
		m.Was, m.Settings = "", canonicalYAML(m.Settings)
		out.Models = append(out.Models, m)
	}
	for _, p := range d.Profiles {
		p.Was, p.Settings = "", canonicalYAML(p.Settings)
		out.Profiles = append(out.Profiles, p)
	}
	out.Normalize()
	return out
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
