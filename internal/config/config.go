// Package config loads and validates the graft configuration file. It does
// not read provider keys: package keys does that when a profile starts.
package config

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// MinContext is the smallest context window a model may declare. It is the
// lower bound of Claude Code's autoCompactWindow, and a smaller window barely
// holds Claude Code's own system prompt and tools.
const MinContext = 100000

// AliasNames are the Claude Code model aliases a profile can redirect, in the
// order graft sets them.
var AliasNames = []string{"opus", "sonnet", "haiku", "fable"}

// Efforts are the values of effort.
var Efforts = []string{"low", "medium", "high", "xhigh", "max"}

// OwnedEnv lists the variables graft manages itself. A profile's env may not
// set them, and graft removes them from the environment it inherits.
var OwnedEnv = []string{
	"ANTHROPIC_BASE_URL",
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_CUSTOM_HEADERS",
	"ANTHROPIC_MODEL",
	"ANTHROPIC_DEFAULT_OPUS_MODEL",
	"ANTHROPIC_DEFAULT_SONNET_MODEL",
	"ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"ANTHROPIC_DEFAULT_FABLE_MODEL",
	"CLAUDE_CODE_SUBAGENT_MODEL",
	"CLAUDE_CODE_MAX_CONTEXT_TOKENS",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX",
	"CLAUDE_CODE_USE_FOUNDRY",
	"CLAUDE_CODE_USE_ANTHROPIC_AWS",
	"CLAUDE_CODE_USE_MANTLE",
}

// Mode says how graft starts Claude Code for a profile.
type Mode int

const (
	// Direct: graft replaces itself with claude, which talks to one provider
	// or to Anthropic.
	Direct Mode = iota
	// Router: graft runs a local router with claude as its child.
	Router
)

func (m Mode) String() string {
	if m == Router {
		return "router"
	}
	return "direct"
}

// Config is a validated configuration file.
type Config struct {
	Path      string // absolute path of the file
	Providers map[string]*Provider
	Models    map[string]*Model
	Defaults  Layer
	Profiles  map[string]*Profile

	ProviderOrder []string // provider names in file order
	ProfileOrder  []string // profile names in file order
}

// Provider is an API that speaks the Anthropic Messages protocol.
type Provider struct {
	Name string
	URL  string // absolute http or https URL
	Key  KeySource
}

// KeySource says where a provider's key comes from. Exactly one field is set.
type KeySource struct {
	Env     string // variable name
	File    string // absolute path
	Command string // run with /bin/sh -c
}

// Model is an external model served by a provider.
type Model struct {
	Name        string
	Provider    string
	ID          string // the model ID the provider expects
	Context     int
	Label       string
	Description string
	Settings    map[string]any // modelSettings fields, passed to Claude Code as is
}

// Layer is what both defaults and profiles may set.
type Layer struct {
	Effort   string
	Settings map[string]any
	Env      map[string]string
	Args     []string
}

// Ref names a model in a profile: a graft model (Model is set) or a Claude
// model of the subscription (Claude is set: an alias or a claude-… ID).
type Ref struct {
	Model  string
	Claude string
}

// External reports whether r names a graft model.
func (r Ref) External() bool { return r.Model != "" }

// String returns the reference as written in the config.
func (r Ref) String() string {
	if r.Model != "" {
		return r.Model
	}
	return r.Claude
}

// Profile is a validated profile; the defaults are not merged into it.
type Profile struct {
	Name         string
	Model        Ref
	Subscription bool
	Picker       []string       // graft model names
	Aliases      map[string]Ref // keys from AliasNames
	Subagents    *Ref
	Layer
}

// ExternalModels returns the graft models p refers to, each once, in this
// order: model, aliases (opus, sonnet, haiku, fable), subagents, picker.
func (c *Config) ExternalModels(p *Profile) []*Model {
	var out []*Model
	seen := map[string]bool{}
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, c.Models[name])
		}
	}
	add(p.Model.Model)
	for _, alias := range AliasNames {
		add(p.Aliases[alias].Model)
	}
	if p.Subagents != nil {
		add(p.Subagents.Model)
	}
	for _, name := range p.Picker {
		add(name)
	}
	return out
}

// ProvidersOf returns the providers of p's external models, each once, in the
// order ExternalModels meets them.
func (c *Config) ProvidersOf(p *Profile) []string {
	var out []string
	for _, m := range c.ExternalModels(p) {
		if !slices.Contains(out, m.Provider) {
			out = append(out, m.Provider)
		}
	}
	return out
}

// ModeOf returns how graft starts p: directly when the profile needs one
// provider and no subscription, or the subscription and no provider;
// through the router otherwise.
func (c *Config) ModeOf(p *Profile) Mode {
	n := len(c.ProvidersOf(p))
	if (p.Subscription && n == 0) || (!p.Subscription && n == 1) {
		return Direct
	}
	return Router
}

// Error is one problem in the config file.
type Error struct {
	File string // as shown to the user
	Line int    // 0 when unknown
	Path string // where in the file, e.g. profiles.mix.picker[1]
	Msg  string
}

func (e Error) Error() string {
	s := e.File
	if e.Line > 0 {
		s += ":" + strconv.Itoa(e.Line)
	}
	if e.Path != "" {
		s += ": " + e.Path
	}
	return s + ": " + e.Msg
}

// Errors is every problem found in the config file.
type Errors []Error

func (es Errors) Error() string {
	lines := make([]string, len(es))
	for i, e := range es {
		lines[i] = e.Error()
	}
	return strings.Join(lines, "\n")
}

// Tildify shows a path under the home directory as ~/….
func Tildify(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}
