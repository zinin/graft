package plan

import (
	"maps"
	"slices"

	"github.com/zinin/graft/internal/config"
)

// pinnedEmpty are variables graft never sets but cancels in --settings, so
// that env in Claude Code's settings files cannot set them either.
var pinnedEmpty = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_MODEL",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX",
	"CLAUDE_CODE_USE_FOUNDRY",
	"CLAUDE_CODE_USE_ANTHROPIC_AWS",
	"CLAUDE_CODE_USE_MANTLE",
}

// pinnedFromEnv are Claude Code switches that README tells users to set in
// env. None of them is a secret, so graft also passes the values that env of
// defaults and profile gives them in --settings, where env in Claude Code's
// settings files cannot override them.
var pinnedFromEnv = []string{
	"CLAUDE_CODE_SUBPROCESS_ENV_SCRUB",
	"CLAUDE_CODE_EFFORT_LEVEL",
	"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC",
	"CLAUDE_CODE_AUTO_MODE_SERVER",
	"ENABLE_CLAUDEAI_MCP_SERVERS",
	"CLAUDE_CODE_DISABLE_AGENT_VIEW",
}

// maxAutoCompactWindow is the largest autoCompactWindow Claude Code accepts.
const maxAutoCompactWindow = 1000000

// settings builds the object passed with --settings, in layers: env, the
// /model rows, per-model settings, then the settings of defaults and profile.
// When the profile, the defaults or the command line set an effort, env
// cancels CLAUDE_CODE_EFFORT_LEVEL, which would silently override --effort,
// unless env of defaults or profile sets the variable. The switches of
// pinnedFromEnv that env of defaults or profile sets go into env too, with
// the same values.
func (v *view) settings(graft []EnvVar) map[string]any {
	env := map[string]any{}
	for _, e := range graft {
		if slices.Contains(addressAndKeys, e.Name) {
			continue
		}
		val := e.Value
		if pv, ok := v.env[e.Name]; ok {
			val = pv
		}
		env[e.Name] = val
	}
	for _, name := range pinnedEmpty {
		env[name] = ""
	}
	if _, ok := v.env["CLAUDE_CODE_EFFORT_LEVEL"]; (v.effort != "" || v.cliEffort) && !ok {
		env["CLAUDE_CODE_EFFORT_LEVEL"] = ""
	}
	for _, name := range pinnedFromEnv {
		if val, ok := v.env[name]; ok {
			env[name] = val
		}
	}
	obj := map[string]any{"env": env}
	if rows := v.pickerRows(); len(rows) > 0 {
		picker := map[string]any{"options": rows}
		if !v.prof.Subscription {
			picker["replaceBuiltInOptions"] = true
		}
		obj["modelPicker"] = picker
	}
	if ms := v.modelSettings(); len(ms) > 0 {
		obj["modelSettings"] = ms
	}
	return merge(obj, v.layer)
}

// pickerRows returns the /model rows: the profile's model when it is a graft
// model, then picker, each once.
func (v *view) pickerRows() []any {
	var names []string
	if v.prof.Model.External() {
		names = append(names, v.prof.Model.Model)
	}
	for _, name := range v.prof.Picker {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	rows := make([]any, 0, len(names))
	for _, name := range names {
		m := v.cfg.Models[name]
		rows = append(rows, map[string]any{
			"model":       v.seen(config.Ref{Model: name}),
			"label":       m.Label,
			"description": m.Description,
		})
	}
	return rows
}

// modelSettings returns modelSettings for the external models. A model with
// a smaller window than CLAUDE_CODE_MAX_CONTEXT_TOKENS compacts at its own
// window; the model's settings go on top.
func (v *view) modelSettings() map[string]any {
	out := map[string]any{}
	limit := v.maxContext()
	for _, m := range v.external {
		entry := map[string]any{}
		if m.Context < limit {
			entry["autoCompactWindow"] = min(m.Context, maxAutoCompactWindow)
		}
		if entry = merge(entry, m.Settings); len(entry) > 0 {
			out[v.seen(config.Ref{Model: m.Name})] = entry
		}
	}
	return out
}

// merge returns base with over applied: mappings merge key by key, and any
// other value from over replaces the one in base. Neither argument changes.
func merge(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	maps.Copy(out, base)
	for k, val := range over {
		bm, baseIsMap := out[k].(map[string]any)
		om, overIsMap := val.(map[string]any)
		if baseIsMap && overIsMap {
			out[k] = merge(bm, om)
			continue
		}
		out[k] = val
	}
	return out
}
