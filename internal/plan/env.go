package plan

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/zinin/graft/internal/config"
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
	if len(v.external) > 0 {
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
	if !v.prof.Subscription {
		out = append(out,
			EnvVar{Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1"},
			EnvVar{Name: "CLAUDE_CODE_AUTO_MODE_SERVER", Value: "0"},
			EnvVar{Name: "ENABLE_CLAUDEAI_MCP_SERVERS", Value: "false"})
	}
	return out
}

// set returns graft's variables with the profile's env applied: a value from
// env replaces graft's, and the rest of env follows in name order.
func (v *view) set(graft []EnvVar) []EnvVar {
	out := make([]EnvVar, 0, len(graft)+len(v.env))
	done := map[string]bool{}
	for _, e := range graft {
		if val, ok := v.env[e.Name]; ok {
			e = EnvVar{Name: e.Name, Value: val}
		}
		out = append(out, e)
		done[e.Name] = true
	}
	for _, name := range slices.Sorted(maps.Keys(v.env)) {
		if !done[name] {
			out = append(out, EnvVar{Name: name, Value: v.env[name]})
		}
	}
	return out
}

// unset returns the inherited variables graft removes: those it manages
// and, when the profile sets an effort, CLAUDE_CODE_EFFORT_LEVEL, which would
// silently override --effort. Variables graft sets again are left out.
func (v *view) unset(environ []string, graft []EnvVar) []string {
	inherited := map[string]bool{}
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		inherited[name] = true
	}
	setAgain := map[string]bool{}
	for _, e := range v.set(graft) {
		setAgain[e.Name] = true
	}
	candidates := slices.Clone(config.OwnedEnv)
	if v.effort != "" {
		candidates = append(candidates, "CLAUDE_CODE_EFFORT_LEVEL")
	}
	var out []string
	for _, name := range candidates {
		if inherited[name] && !setAgain[name] {
			out = append(out, name)
		}
	}
	return out
}
