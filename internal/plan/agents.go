package plan

import (
	"fmt"
	"strings"

	"github.com/zinin/graft/internal/config"
)

// agents returns the subagents that Claude Code gets with --agents, by name,
// and what of args, those of defaults, profile and command line, goes to
// claude besides them. There is an agent on each external model when the
// session has models to choose from: next to the subscription, or when there
// are two external models. Without agents the result is nil.
func (v *view) agents(args []string) (agents map[string]any, rest []string) {
	if len(v.external) == 0 || len(v.external) == 1 && !v.prof.Subscription {
		return nil, args
	}
	agents = map[string]any{}
	for _, m := range v.external {
		def := map[string]any{
			"description": agentDescription(m),
			"prompt":      agentPrompt(m, v.prof.Subscription),
			"model":       v.seen(config.Ref{Model: m.Name}),
		}
		// Without a level of its own, an agent on another model than the
		// session's runs at that model's default, not at the session's level.
		if v.effort != "" {
			def["effort"] = v.effort
		}
		agents[m.Name] = def
	}
	return agents, args
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
