package plan

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/zinin/graft/internal/config"
)

// agents returns the subagents that Claude Code gets with --agents, by name,
// and what of args, those of defaults, profile and command line, goes to
// claude besides them. There is an agent on each external model when the
// session has models to choose from: next to the subscription, or when there
// are two external models. The user's own agents, of an --agents in args,
// join them, and win over graft's of the same name: of two --agents Claude
// Code would take only the last. The result is nil without agents of graft,
// and when the user's are no JSON object, such as the path of a file, which
// Claude Code takes with -p; args are then as they were.
func (v *view) agents(args []string) (agents map[string]any, rest []string) {
	if len(v.external) == 0 || len(v.external) == 1 && !v.prof.Subscription {
		return nil, args
	}
	rest, values := takeOption(args, "--agents")
	var own map[string]json.RawMessage
	if n := len(values); n > 0 {
		// null decodes into a nil map without an error.
		if err := json.Unmarshal([]byte(values[n-1]), &own); err != nil || own == nil {
			return nil, args
		}
	}
	effort := v.startEffort(rest)
	agents = map[string]any{}
	for _, m := range v.external {
		def := map[string]any{
			"description": agentDescription(m),
			"prompt":      agentPrompt(m, v.prof.Subscription),
			"model":       v.seen(config.Ref{Model: m.Name}),
		}
		// Without a level of its own, an agent on another model than the
		// session's runs at that model's default, not at the session's level.
		if effort != "" {
			def["effort"] = effort
		}
		agents[m.Name] = def
	}
	for name, def := range own {
		agents[name] = def
	}
	return agents, rest
}

// startEffort returns the level the session starts at: that of the last
// --effort in args, else that of the profile or the defaults. It is "" for
// no level, and for one that a definition of an agent cannot have, such as
// ultracode: Claude Code does not start with a definition it cannot read.
func (v *view) startEffort(args []string) string {
	_, levels := takeOption(args, "--effort")
	if len(levels) == 0 {
		return v.effort
	}
	if last := levels[len(levels)-1]; slices.Contains(config.Efforts, last) {
		return last
	}
	return ""
}

// agentDescription returns what Claude reads of the agent on m in its list
// of agents: which model it runs on and when to use it.
func agentDescription(m *config.Model) string {
	return fmt.Sprintf("General-purpose agent on %[1]s: %[2]s. Use it when the user or their instructions ask for this model. "+
		"Call it without `model`: a `model` in the call would replace %[1]s.",
		oneLine(m.Label), strings.TrimSuffix(oneLine(m.Description), "."))
}

// agentPrompt returns the system prompt of the agent on m. A model in the
// call that starts an agent replaces the agent's own. With the subscription
// that is a Claude model, so there the prompt tells such a model to leave the
// task alone and say so. Without it, the call can only name external models.
func agentPrompt(m *config.Model, subscription bool) string {
	const task = "the task in the message you got with the tools you have. Complete it fully: don't gold-plate, " +
		"but don't leave it half-done. You are the agent for this task: do the work yourself, and do not hand the whole " +
		"task to another agent. When you are done, reply with a concise report of what you did and what you found: " +
		"the agent that started you reads only your reply."
	label := oneLine(m.Label)
	if !subscription {
		return fmt.Sprintf("This agent runs on %s.\n\nDo ", label) + task
	}
	return fmt.Sprintf("This agent runs on %[1]s and on no other model. Before anything else, look at the model that your "+
		"environment section says you are powered by. If its ID starts with `claude-`, the call that started you replaced "+
		"the model by mistake: use no tool, leave the task undone, and reply with this one line only: "+
		"\"Wrong model: this agent runs on %[1]s. Call it again without `model`.\"\n\nOtherwise, do ", label) + task
}

// oneLine returns s with one space between its words, so that it stays on
// its line of Claude's list of agents.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// takeOption returns args without the option name, which takes a value, and
// the values it has there, as "name value" or "name=value", before a "--"
// that ends the options. A name with no value after it stays, for claude to
// report.
func takeOption(args []string, name string) (rest, values []string) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			return append(rest, args[i:]...), values
		}
		value, ok := strings.CutPrefix(args[i], name+"=")
		if !ok && args[i] == name && i+1 < len(args) {
			i++
			value, ok = args[i], true
		}
		if ok {
			values = append(values, value)
		} else {
			rest = append(rest, args[i])
		}
	}
	return rest, values
}
