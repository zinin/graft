// Package plan turns a profile into a launch plan: the arguments,
// environment and settings Claude Code gets. Build does no I/O; the files
// it checks come from LoadClaudeSettings.
package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/zinin/graft/internal/config"
)

// Plan says how to start Claude Code for a profile.
type Plan struct {
	Profile      string
	Mode         config.Mode
	Subscription bool           // whether the session uses the claude.ai login
	Providers    []string       // providers of the profile's external models
	Routes       []Route        // in the router mode, where each external model goes
	Args         []string       // arguments for claude, without the program name
	Unset        []string       // inherited variables graft removes and does not set again
	Set          []EnvVar       // variables graft sets, in this order
	Settings     map[string]any // the object passed with --settings
}

// Route is where the router sends the requests of an external model.
type Route struct {
	Model    string // the graft model name, which Claude Code sends
	Provider string
	URL      string // the provider's base URL
	ID       string // the model ID the provider expects
}

// EnvVar is a variable graft sets. The launcher fills in the value at start
// when KeyOf names a provider, with that provider's key, or when Session is
// set, with a value of the running router; Value is empty then.
type EnvVar struct {
	Name    string
	Value   string
	KeyOf   string
	Session Session
}

// Session names a value that only the running router knows.
type Session int

const (
	SessionURL    Session = iota + 1 // the router's address, http://127.0.0.1:<port>
	SessionHeader                    // the header with the session secret
)

// Input is what Build needs.
type Input struct {
	Config         *config.Config
	Profile        string
	Args           []string       // command-line arguments after the profile name
	Environ        []string       // the environment graft runs in, as from os.Environ
	ClaudeSettings []SettingsFile // from LoadClaudeSettings
}

// Build makes the plan for in.Profile.
func Build(in Input) (*Plan, error) {
	cfg := in.Config
	prof, ok := cfg.Profiles[in.Profile]
	if !ok {
		return nil, fmt.Errorf("no profile %q in %s", in.Profile, config.Tildify(cfg.Path))
	}
	if hasOption(in.Args, "--settings") {
		return nil, errors.New("--settings is managed by graft: put Claude Code settings into the profile's settings")
	}
	v := newView(cfg, prof)
	if len(v.external) > 0 {
		if found := Conflicts(in.ClaudeSettings); len(found) > 0 {
			return nil, fmt.Errorf("profile %q sends requests to an external provider, "+
				"but Claude Code settings would override its address or key:\n  %s\nremove these keys first",
				prof.Name, strings.Join(found, "\n  "))
		}
	}
	if v.mode == config.Router {
		// The profile's own NO_PROXY goes into graft's, which keeps 127.0.0.1.
		v.noProxy = noProxy(in.Environ, v.env, in.ClaudeSettings)
		delete(v.env, "NO_PROXY")
		delete(v.env, "no_proxy")
	}
	graft := v.graftEnv()
	settings := v.settings(graft)
	encoded, err := marshal(settings)
	if err != nil {
		return nil, err
	}
	return &Plan{
		Profile:      prof.Name,
		Mode:         v.mode,
		Subscription: prof.Subscription,
		Providers:    cfg.ProvidersOf(prof),
		Routes:       v.routes(),
		Args:         v.args(in.Args, encoded),
		Unset:        v.unset(in.Environ, graft),
		Set:          v.set(graft),
		Settings:     settings,
	}, nil
}

// view is a profile with the defaults merged in.
type view struct {
	cfg      *config.Config
	prof     *config.Profile
	mode     config.Mode
	external []*config.Model
	effort   string
	layer    map[string]any // settings of defaults and profile
	env      map[string]string
	extra    []string // args of defaults and profile
	noProxy  string   // in the router mode, the hosts Claude Code reaches past a proxy
}

func newView(cfg *config.Config, prof *config.Profile) *view {
	d := cfg.Defaults
	v := &view{
		cfg:      cfg,
		prof:     prof,
		mode:     cfg.ModeOf(prof),
		external: cfg.ExternalModels(prof),
		effort:   prof.Effort,
		layer:    merge(d.Settings, prof.Settings),
		env:      map[string]string{},
		extra:    append(slices.Clone(d.Args), prof.Args...),
	}
	if v.effort == "" {
		v.effort = d.Effort
	}
	maps.Copy(v.env, d.Env)
	maps.Copy(v.env, prof.Env)
	return v
}

// seen returns the model name Claude Code sees for r: for a graft model, its
// name when the router stands between and the provider's id otherwise; a
// Claude model as written.
func (v *view) seen(r config.Ref) string {
	switch {
	case !r.External():
		return r.Claude
	case v.mode == config.Router:
		return r.Model
	}
	return v.cfg.Models[r.Model].ID
}

// routes returns where the router sends each external model, in the router
// mode.
func (v *view) routes() []Route {
	if v.mode != config.Router {
		return nil
	}
	out := make([]Route, len(v.external))
	for i, m := range v.external {
		out[i] = Route{Model: m.Name, Provider: m.Provider, URL: v.cfg.Providers[m.Provider].URL, ID: m.ID}
	}
	return out
}

// alias returns where Claude Code's alias name points, when graft sets it.
func (v *view) alias(name string) (config.Ref, bool) {
	if r, ok := v.prof.Aliases[name]; ok {
		return r, true
	}
	if !v.prof.Subscription {
		return v.prof.Model, true
	}
	return config.Ref{}, false
}

// subagents returns the model for subagents, when graft sets it.
func (v *view) subagents() (config.Ref, bool) {
	if v.prof.Subagents != nil {
		return *v.prof.Subagents, true
	}
	if !v.prof.Subscription {
		return v.prof.Model, true
	}
	return config.Ref{}, false
}

// maxContext is the largest window among the external models.
func (v *view) maxContext() int {
	n := 0
	for _, m := range v.external {
		n = max(n, m.Context)
	}
	return n
}

// args returns the arguments for claude: graft's --model, --effort and
// --settings, then the args of defaults and profile, then the command line.
// A --model or --effort on the command line replaces graft's.
func (v *view) args(cli []string, settings string) []string {
	var out []string
	if !hasOption(cli, "--model") {
		out = append(out, "--model", v.seen(v.prof.Model))
	}
	if v.effort != "" && !hasOption(cli, "--effort") {
		out = append(out, "--effort", v.effort)
	}
	out = append(out, "--settings", settings)
	out = append(out, v.extra...)
	return append(out, cli...)
}

// hasOption reports whether args set the option name, as "name value" or
// "name=value", before a "--" that ends the options.
func hasOption(args []string, name string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}

// marshal encodes the settings compactly and without HTML escaping.
func marshal(settings map[string]any) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(settings); err != nil {
		return "", fmt.Errorf("encode settings: %w", err)
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
