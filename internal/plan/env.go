package plan

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/osenv"
)

// aliasVars maps each alias to the variable that redirects it.
var aliasVars = map[string]string{
	"opus":   "ANTHROPIC_DEFAULT_OPUS_MODEL",
	"sonnet": "ANTHROPIC_DEFAULT_SONNET_MODEL",
	"haiku":  "ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"fable":  "ANTHROPIC_DEFAULT_FABLE_MODEL",
}

// graftEnv returns the variables graft sets for the profile, before the
// profile's own env, in a fixed order.
func (v *view) graftEnv() []EnvVar {
	var out []EnvVar
	switch {
	case v.mode == config.Router:
		out = append(out,
			EnvVar{Name: "ANTHROPIC_BASE_URL", Session: SessionURL},
			EnvVar{Name: "ANTHROPIC_CUSTOM_HEADERS", Session: SessionHeader})
		if !v.prof.Subscription {
			// A stand-in: the router gives each provider its own key, and
			// without a token Claude Code would ask for the claude.ai login.
			out = append(out, EnvVar{Name: "ANTHROPIC_AUTH_TOKEN", Value: "graft"})
		}
	case len(v.external) > 0:
		// In the direct mode every external model has the same provider.
		prov := v.cfg.Providers[v.external[0].Provider]
		out = append(out,
			EnvVar{Name: "ANTHROPIC_BASE_URL", Value: prov.URL},
			EnvVar{Name: "ANTHROPIC_AUTH_TOKEN", KeyOf: prov.Name})
	}
	for _, name := range config.AliasNames {
		if r, ok := v.alias(name); ok {
			out = append(out, EnvVar{Name: aliasVars[name], Value: v.seen(r)})
		}
	}
	if r, ok := v.subagents(); ok {
		out = append(out, EnvVar{Name: "CLAUDE_CODE_SUBAGENT_MODEL", Value: v.seen(r)})
	}
	if len(v.external) > 0 {
		out = append(out,
			EnvVar{Name: "CLAUDE_CODE_MAX_CONTEXT_TOKENS", Value: strconv.Itoa(v.maxContext())},
			// A background session would run without the address and the key.
			EnvVar{Name: "CLAUDE_CODE_DISABLE_AGENT_VIEW", Value: "1"})
	}
	if v.noProxy != "" {
		// Through a proxy, Claude Code's requests would not reach the router
		// or a provider on this machine, and the proxy would see the key.
		out = append(out,
			EnvVar{Name: "NO_PROXY", Value: v.noProxy},
			EnvVar{Name: "no_proxy", Value: v.noProxy})
	}
	if !v.prof.Subscription {
		out = append(out,
			EnvVar{Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1"},
			EnvVar{Name: "CLAUDE_CODE_AUTO_MODE_SERVER", Value: "0"},
			EnvVar{Name: "ENABLE_CLAUDEAI_MCP_SERVERS", Value: "false"})
	}
	return out
}

// noProxy returns the hosts a session keeps away from proxies: those of
// NO_PROXY and no_proxy in the environment, in env of defaults and profile
// and in env of the Claude Code settings files, then host, each once. On
// Windows, where the two are one variable, they count in any case.
func noProxy(environ []string, env map[string]string, files []SettingsFile, host string) string {
	var lists []string
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		lists = append(lists, lookup(environ, name))
		for _, k := range slices.Sorted(maps.Keys(env)) {
			if osenv.Equal(k, name) {
				lists = append(lists, env[k])
			}
		}
		for _, f := range files {
			fileEnv, _ := f.Data["env"].(map[string]any)
			for _, k := range slices.Sorted(maps.Keys(fileEnv)) {
				if osenv.Equal(k, name) {
					s, _ := fileEnv[k].(string)
					lists = append(lists, s)
				}
			}
		}
	}
	var hosts []string
	for _, list := range append(lists, host) {
		for _, h := range strings.Split(list, ",") {
			if h = strings.TrimSpace(h); h != "" && !slices.Contains(hosts, h) {
				hosts = append(hosts, h)
			}
		}
	}
	return strings.Join(hosts, ",")
}

// set returns graft's variables with the profile's env applied: a value from
// env replaces graft's, and the rest of env follows in name order.
func (v *view) set(graft []EnvVar) []EnvVar {
	out := make([]EnvVar, 0, len(graft)+len(v.env))
	used := map[string]bool{}
	for _, e := range graft {
		if k, ok := envKey(v.env, e.Name); ok {
			e = EnvVar{Name: e.Name, Value: v.env[k]}
			used[k] = true
		}
		out = append(out, e)
	}
	for _, name := range slices.Sorted(maps.Keys(v.env)) {
		if !used[name] {
			out = append(out, EnvVar{Name: name, Value: v.env[name]})
		}
	}
	return out
}

// unset returns the inherited variables graft removes: those it manages
// and, when the profile, the defaults or the command line set an effort,
// CLAUDE_CODE_EFFORT_LEVEL, which would silently override --effort.
// Variables graft sets again are left out.
func (v *view) unset(environ []string, graft []EnvVar) []string {
	var inherited []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		inherited = append(inherited, name)
	}
	var setAgain []string
	for _, e := range v.set(graft) {
		setAgain = append(setAgain, e.Name)
	}
	candidates := slices.Clone(config.OwnedEnv)
	if v.effort != "" || v.cliEffort {
		candidates = append(candidates, "CLAUDE_CODE_EFFORT_LEVEL")
	}
	var out []string
	for _, name := range candidates {
		same := func(n string) bool { return osenv.Equal(n, name) }
		if slices.ContainsFunc(inherited, same) && !slices.ContainsFunc(setAgain, same) {
			out = append(out, name)
		}
	}
	return out
}

// envKey returns the key of env that names the variable name: name itself,
// else, on Windows, where names differ in case only, the first such key in
// sorted order.
func envKey(env map[string]string, name string) (string, bool) {
	if _, ok := env[name]; ok {
		return name, true
	}
	for _, k := range slices.Sorted(maps.Keys(env)) {
		if osenv.Equal(k, name) {
			return k, true
		}
	}
	return "", false
}

// envValue returns the value of the variable name in the env of defaults
// and profile; see envKey.
func (v *view) envValue(name string) (string, bool) {
	k, ok := envKey(v.env, name)
	return v.env[k], ok
}
