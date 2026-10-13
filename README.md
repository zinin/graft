# graft

Graft other models onto Claude Code: run whole sessions on GPT, GLM, Kimi or DeepSeek, or mix them with your Claude subscription in one `/model` picker.

Claude Code works with any provider that speaks the Anthropic Messages API once `ANTHROPIC_BASE_URL` points at it. A working session takes a dozen environment variables, though, usually kept in a shell function per model. graft keeps providers, models and profiles in one config file and starts Claude Code with the environment and settings of a profile:

```sh
graft glm             # a whole session on GLM
graft glm --resume    # arguments after the profile go to claude
```

> **Status.** graft starts sessions on one provider. Profiles that mix providers, or add external models to a session on your Claude subscription, need a local router that is still in development: graft reports such profiles instead of starting them.

## Install

graft runs on Linux and macOS with Claude Code 2.1.288 or later. Build it with Go 1.26:

```sh
go install github.com/zinin/graft/cmd/graft@latest
```

From a clone, `make install` puts the binary into `~/.local/bin`.

## Quick start

1. Put your provider's key where graft can read it, for example in a file that only you can read:

   ```sh
   mkdir -p ~/.config/graft/keys
   (umask 077; cat > ~/.config/graft/keys/ollama)   # paste the key, then press Ctrl-D
   ```

2. Write `~/.config/graft/config.yaml`:

   ```yaml
   providers:
     ollama:
       url: https://ollama.com
       key: { file: ~/.config/graft/keys/ollama }

   models:
     glm:
       provider: ollama
       id: glm-5.3-flash
       context: 1048576

   profiles:
     glm:
       model: glm
       effort: max
       settings: { autoCompactEnabled: true }
   ```

3. Check the config and see what graft would start:

   ```sh
   graft check
   graft --dry-run glm
   ```

4. Start the session: `graft glm`.

[config.example.yaml](config.example.yaml) shows more.

## How it works

For a profile on one provider, graft replaces itself with `claude` and gives it:

- `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN`: the provider's address and key;
- `ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU,FABLE}_MODEL` and `CLAUDE_CODE_SUBAGENT_MODEL`: every alias and every subagent run on the profile's model, so `fable` never turns into `claude-fable-5-1`;
- `CLAUDE_CODE_MAX_CONTEXT_TOKENS`: the model's window;
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `CLAUDE_CODE_AUTO_MODE_SERVER=0`, `ENABLE_CLAUDEAI_MCP_SERVERS=false` and `CLAUDE_CODE_DISABLE_AGENT_VIEW=1`;
- `--model`, `--effort`, and `--settings` with the profile's settings, the `/model` rows and the per-model settings.

Settings passed with `--settings` override your settings files for this session only. graft never writes to Claude Code's files, to your shell files or to its own config.

graft removes the variables it manages from the environment it inherits. It also cancels `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL` and the `CLAUDE_CODE_USE_*` provider switches in `--settings`, so a key from your shell or from Claude Code's settings never reaches the provider. `graft --dry-run PROFILE` prints all of it.

## Configuration

The config file is `~/.config/graft/config.yaml`, or `$XDG_CONFIG_HOME/graft/config.yaml` when that variable is set; `--config FILE` or `GRAFT_CONFIG` picks another file. An unknown field is an error, and graft reports every problem of the file at once, each with its line.

### providers

| Field | Meaning |
| --- | --- |
| `url` | the base URL of the Anthropic Messages API; Claude Code appends `/v1/messages` |
| `key` | where the key comes from: exactly one of `env: NAME`, `file: PATH` or `command: "CMD"` |

The key itself never goes into the config. A key file is trimmed of white space at the ends. A command runs with `/bin/sh -c` in your terminal, so a password manager can ask for a password: `command: "pass show ollama"`. graft reads only the keys of the profile it starts.

### models

| Field | Default | Meaning |
| --- | --- | --- |
| `provider` | | a provider name |
| `id` | the model name | the model ID the provider expects |
| `context` | | the context window in tokens, 100000 or more |
| `label` | the model name | the `/model` row |
| `description` | `via <provider>` | its second line |
| `settings` | | Claude Code `modelSettings` for this model, such as `maxEffortLevel` |

A model name may not start with `claude` or be a Claude Code alias: `opus`, `sonnet`, `haiku`, `fable`, `default`, `best`, `opusplan`, `inherit`.

### profiles

| Field | Default | Meaning |
| --- | --- | --- |
| `model` | | the starting model: a model of this config or, with the subscription, a Claude model such as `opus` or `claude-opus-5-5` |
| `subscription` | true when `model` is a Claude model | whether the session uses your claude.ai login |
| `picker` | | more models for `/model` |
| `aliases` | see below | where `opus`, `sonnet`, `haiku` and `fable` point; `sonnet` also runs the auto mode classifier, `haiku` the background tasks |
| `subagents` | see below | the model of subagents that have no model of their own |
| `effort` | | `low`, `medium`, `high`, `xhigh` or `max`, passed as `--effort`; `/effort` changes it in the session |
| `settings` | | Claude Code settings for this session, such as `autoCompactEnabled: true` |
| `env` | | more environment variables |
| `args` | | more arguments for `claude` |

Without the subscription, the aliases and `subagents` you do not set point at `model`, and `/model` lists only the profile's models. With the subscription, they stay Claude models, and the external models follow the built-in rows of `/model`.

`defaults` takes `effort`, `settings`, `env` and `args` for every profile. A profile's values win; the `args` of `defaults` come first.

To lock the effort so that `/effort` cannot change it, set `env: { CLAUDE_CODE_EFFORT_LEVEL: max }`.

## Commands

| Command | What it does |
| --- | --- |
| `graft PROFILE [ARGS...]` | starts Claude Code; ARGS go to `claude` |
| `graft --dry-run PROFILE [ARGS...]` | prints the environment, the command and the settings, with keys shown as fingerprints |
| `graft list` | lists the profiles |
| `graft check` | checks the config, the keys, `claude` and Claude Code's settings files |

## Security

- Keys never appear on the command line, in `--settings` or in graft's output: `--dry-run` and `check` show the first 12 hex digits of a key's SHA-256.
- Claude Code passes its environment to the commands it runs, to hooks and to MCP servers, so they can read the provider's key. To strip credentials from them, add `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB: "1"` to the profile's `env`. On Linux this also runs Bash commands in their own PID namespace, where `ps` and `kill` do not see your other processes.
- graft refuses to start a profile with external models when Claude Code's settings set `apiKeyHelper`, or an `env` with `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` or `ANTHROPIC_CUSTOM_HEADERS`. graft reads `~/.claude/settings.json` (or `$CLAUDE_CONFIG_DIR/settings.json`) and `.claude/settings.json` and `.claude/settings.local.json` in the current directory. Such settings would send the provider's key elsewhere, or another key to the provider.
- Background sessions (`/background`, `claude --bg`) are off in profiles with external models, because a background session may lose the provider's address and key. Set `CLAUDE_CODE_DISABLE_AGENT_VIEW: "0"` in `env` at your own risk.

## Troubleshooting

| Message or symptom | What to do |
| --- | --- |
| `profile "x" needs the router, which is not implemented yet` | The profile mixes providers, or adds external models to a subscription session. Until the router lands, keep one provider per profile. |
| `… Claude Code settings would override its address or key` | Remove the keys that graft names from those settings files, or start the profile from another directory if they are project files. |
| `provider x: env NAME is not set` | Export the variable in the shell that runs graft, or switch the key to `file` or `command`. |
| `key file … is readable by other users` | `chmod 600` the file. |
| Claude Code shows a 200k window or the wrong model | Run `graft --dry-run PROFILE` and check `CLAUDE_CODE_MAX_CONTEXT_TOKENS` and the aliases; a `--model` on the command line replaces the profile's model. |

## Limitations

- Anthropic does not support Claude Code on non-Claude models; it works, without support.
- graft speaks only the Anthropic Messages API. For an OpenAI-compatible provider, put CLIProxyAPI, LiteLLM or a similar gateway in between.
- Claude Code shows `unrecognized_model` and a model catalog warning for external model IDs.
- Switching models in the middle of a conversation resets the prompt cache.

## License

[MIT](LICENSE)
