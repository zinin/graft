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
	Profile   string
	Mode      config.Mode
	Providers []string       // providers of the profile's external models
	Args      []string       // arguments for claude, without the program name
	Unset     []string       // inherited variables graft removes and does not set again
	Set       []EnvVar       // variables graft sets, in this order
	Settings  map[string]any // the object passed with --settings
}

// EnvVar is a variable graft sets. When KeyOf names a provider, the value is
// that provider's key, which the launcher reads at start; Value is empty then.
type EnvVar struct {
	Name  string
	Value string
	KeyOf string
}

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
	mode := cfg.ModeOf(prof)
	if mode == config.Router {
		return nil, fmt.Errorf("profile %q needs the router, which is not implemented yet", prof.Name)
	}
	v := newView(cfg, prof)
	if len(v.external) > 0 {
		if found := Conflicts(in.ClaudeSettings); len(found) > 0 {
			return nil, fmt.Errorf("profile %q sends requests to an external provider, "+
				"but Claude Code settings would override its address or key:\n  %s\nremove these keys first",
				prof.Name, strings.Join(found, "\n  "))
		}
	}
	graft := v.graftEnv()
	settings := v.settings(graft)
	encoded, err := marshal(settings)
	if err != nil {
		return nil, err
	}
	return &Plan{
		Profile:   prof.Name,
		Mode:      mode,
		Providers: cfg.ProvidersOf(prof),
		Args:      v.args(in.Args, encoded),
		Unset:     v.unset(in.Environ, graft),
		Set:       v.set(graft),
		Settings:  settings,
	}, nil
}

// view is a profile with the defaults merged in.
type view struct {
	cfg      *config.Config
	prof     *config.Profile
	external []*config.Model
	effort   string
	layer    map[string]any // settings of defaults and profile
	env      map[string]string
	extra    []string // args of defaults and profile
}

func newView(cfg *config.Config, prof *config.Profile) *view {
	d := cfg.Defaults
	v := &view{
		cfg:      cfg,
		prof:     prof,
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

// seen returns the model name Claude Code sees for r: the provider's id of a
// graft model, or the Claude model as written.
func (v *view) seen(r config.Ref) string {
	if r.External() {
		return v.cfg.Models[r.Model].ID
	}
	return r.Claude
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
