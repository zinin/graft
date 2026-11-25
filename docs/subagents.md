# Subagents

[graft](../README.md) › Subagents

The Agent tool of Claude Code takes only Claude's aliases as a subagent's model. So when a session has models to choose from, with the subscription or with two external models, graft makes each external model of the profile a subagent type under the model's name. Claude sees these agents with the `label` and `description` of their models, and starts one when you or your instructions ask for that model:

> Give the implementation tasks to sol and review its work yourself.

Opus can thus plan and review while GPT implements. graft passes the agents with `--agents`, for this session only, and writes nothing to `~/.claude/agents`. [GPT next to Claude](gpt-next-to-claude.md) shows such a session.

## Which agents there are

- An agent is general-purpose: it has all the tools, and its role comes with its task. A standing role for the main model goes into CLAUDE.md, into a plan, or into the profile's `args` as `--append-system-prompt`: see the profile `lead` in [config.example.yaml](../config.example.yaml).
- To let Claude choose among several models, say in `description` what each is good for.
- To hide an agent, deny it in the profile: `settings: { permissions: { deny: ["Agent(sol)"] } }`. A list in the profile replaces the one in `defaults`, so repeat its rules. To start without any agent of graft, pass an empty `--agents`: `args: [--agents, ""]`.
- An `--agents` of your own, in `args` or on the command line, joins graft's and wins where a name is the same. The path of a file, which Claude Code takes with `-p`, replaces graft's.
- An agent of graft replaces one of the same name in `~/.claude/agents` or `.claude/agents`, or one built into Claude Code, such as `Explore`.

## Their model and effort

- A `model` in the call replaces the agent's own. The agent's description asks Claude to leave it out, and with the subscription the agent's prompt tells a Claude model in its place to stop and say so.
- An agent runs at the level the session starts at: the profile's `effort`, or your `--effort`, in `args` or on the command line. It does not go above its model's `maxEffortLevel`, and `/effort` and `CLAUDE_CODE_SUBAGENT_EFFORT` do not change it. When there is no such level, or it is one that an agent cannot have, such as `ultracode`, Claude Code picks the agent's level, which these two may change. You can ask Claude to start the agent at another level, and `CLAUDE_CODE_EFFORT_LEVEL` overrides every level.
- `CLAUDE_CODE_SUBAGENT_MODEL_FORCE` makes Claude Code ignore the models of agents.
- The fallback models of Claude Code, from `--fallback-model` or the `fallbackModel` of its settings, count for agents too: when an agent's model is overloaded or unavailable, the agent goes on with one of them, with the subscription perhaps a Claude model, and its result says so.

## What you see and what it costs

- `/tasks` shows the model of each subagent, and behind the router the log shows each of its requests.
- While an agent runs, Claude Code asks about every 30 seconds for a line on its progress, which the agent's row shows. The agent's own model writes it from the agent's conversation, so on an external model these requests spend that provider's quota. `env: { CLAUDE_CODE_DISABLE_AGENT_PROGRESS_SUMMARIES: "1" }` turns the lines off, for Claude's subagents too.
