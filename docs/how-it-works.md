# How it works

[graft](../README.md) › How it works

graft starts a profile in one of two modes. `graft list` shows which.

**Direct.** When the external models of a profile are all at one provider and the profile does not use the subscription, graft replaces itself with `claude` and gives it:

- `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN`: the provider's address and key;
- `ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU,FABLE}_MODEL` and `CLAUDE_CODE_SUBAGENT_MODEL`: every alias and every subagent without a model of its own run on the profile's model, so `fable` never turns into `claude-fable-5-1`;
- `CLAUDE_CODE_MAX_CONTEXT_TOKENS`: the model's window;
- when the provider runs on this machine, its host joins `NO_PROXY` and `no_proxy`, so that a proxy neither breaks the session nor sees the key;
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `CLAUDE_CODE_AUTO_MODE_SERVER=0`, `ENABLE_CLAUDEAI_MCP_SERVERS=false` and `CLAUDE_CODE_DISABLE_AGENT_VIEW=1`;
- `--model`, `--effort`, and `--settings` with the profile's settings, the `/model` rows and the per-model settings;
- `--agents` with a subagent on each external model, when there are two or more: see [Subagents](subagents.md).

A profile on the subscription alone starts directly too: it is Claude Code with the profile's settings.

**Router.** A profile with models at several providers, or with external models next to the subscription, runs behind a router. graft opens it on `127.0.0.1` at a random port and starts `claude` as its child:

- `ANTHROPIC_BASE_URL` points at the router, and `ANTHROPIC_CUSTOM_HEADERS` gives Claude Code a secret of the session, without which the router serves nothing;
- Claude Code sees the model names of the config, and the router sends each request to the provider of its model, with the provider's key and the model ID the provider expects;
- with the subscription, Claude Code keeps your claude.ai login, and every request that is not for an external model goes to `https://api.anthropic.com` as it came; without the subscription, `ANTHROPIC_AUTH_TOKEN` is a stand-in, and other models are refused;
- `127.0.0.1` joins `NO_PROXY` and `no_proxy`, so that Claude Code reaches the router past any proxy;
- `--agents` gives Claude a subagent on each external model of the profile: see [Subagents](subagents.md);
- without the subscription, the same flags as in the direct mode: `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `CLAUDE_CODE_AUTO_MODE_SERVER=0`, `ENABLE_CLAUDEAI_MCP_SERVERS=false` and `CLAUDE_CODE_DISABLE_AGENT_VIEW=1`; with the subscription, only `CLAUDE_CODE_DISABLE_AGENT_VIEW=1`, since the auto mode, the claude.ai connectors and the background traffic work with your login. If you need the others, set them in the `env` of the profile or of `defaults`.

graft passes `SIGTERM` and `SIGHUP` on to `claude` and exits with its exit code; the router stops with `claude`. The router writes a line for each request to `~/.local/state/graft/graft.log`, or to `$XDG_STATE_HOME/graft/graft.log` when that variable is set: the model, where the request went, the status, how long it took. The log holds no headers, bodies, keys or secret.

In both modes, settings passed with `--settings` override your settings files for this session only. graft never writes to Claude Code's files or to your shell files. Only `graft ui` writes its config, when you save, and a key file, when you save a key.

graft removes the variables it manages from the environment it inherits. It also cancels `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL` and the `CLAUDE_CODE_USE_*` provider switches in `--settings`, so a key from your shell or from Claude Code's settings never reaches a provider. `graft --dry-run PROFILE` prints all of it.
