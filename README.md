# graft

Graft other models onto Claude Code: run whole sessions on GPT, GLM, Kimi or DeepSeek, or mix them with your Claude subscription in one `/model` picker.

Claude Code works with any provider that speaks the Anthropic Messages API once `ANTHROPIC_BASE_URL` points at it. A working session takes a dozen environment variables, though, usually kept in a shell function per model, and all the requests of a session go to that one address. graft keeps providers, models and profiles in one config file and starts Claude Code with the environment and settings of a profile. When a profile mixes providers, or adds external models to a session on your Claude subscription, a local router sends each request where its model lives:

```sh
graft glm             # a whole session on GLM
graft glm --resume    # arguments after the profile go to claude
graft mix             # Opus on your subscription, with GPT and GLM in /model
```

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

[config.example.yaml](config.example.yaml) shows more, including profiles that mix providers and the subscription.

## How it works

graft starts a profile in one of two modes. `graft list` shows which.

**Direct.** When the external models of a profile are all at one provider and the profile does not use the subscription, graft replaces itself with `claude` and gives it:

- `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN`: the provider's address and key;
- `ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU,FABLE}_MODEL` and `CLAUDE_CODE_SUBAGENT_MODEL`: every alias and every subagent run on the profile's model, so `fable` never turns into `claude-fable-5-1`;
- `CLAUDE_CODE_MAX_CONTEXT_TOKENS`: the model's window;
- when the provider runs on this machine, its host joins `NO_PROXY` and `no_proxy`, so that a proxy neither breaks the session nor sees the key;
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `CLAUDE_CODE_AUTO_MODE_SERVER=0`, `ENABLE_CLAUDEAI_MCP_SERVERS=false` and `CLAUDE_CODE_DISABLE_AGENT_VIEW=1`;
- `--model`, `--effort`, and `--settings` with the profile's settings, the `/model` rows and the per-model settings.

A profile on the subscription alone starts directly too: it is Claude Code with the profile's settings.

**Router.** A profile with models at several providers, or with external models next to the subscription, runs behind a router. graft opens it on `127.0.0.1` at a random port and starts `claude` as its child:

- `ANTHROPIC_BASE_URL` points at the router, and `ANTHROPIC_CUSTOM_HEADERS` gives Claude Code a secret of the session, without which the router serves nothing;
- Claude Code sees the model names of the config, and the router sends each request to the provider of its model, with the provider's key and the model ID the provider expects;
- with the subscription, Claude Code keeps your claude.ai login, and every request that is not for an external model goes to `https://api.anthropic.com` as it came; without the subscription, `ANTHROPIC_AUTH_TOKEN` is a stand-in, and other models are refused;
- `127.0.0.1` joins `NO_PROXY` and `no_proxy`, so that Claude Code reaches the router past any proxy.

graft passes `SIGTERM` and `SIGHUP` on to `claude` and exits with its exit code; the router stops with `claude`. The router writes a line for each request to `~/.local/state/graft/graft.log`, or to `$XDG_STATE_HOME/graft/graft.log` when that variable is set: the model, where the request went, the status, how long it took. The log holds no headers, bodies, keys or secret.

In both modes, settings passed with `--settings` override your settings files for this session only. graft never writes to Claude Code's files, to your shell files or to its own config.

graft removes the variables it manages from the environment it inherits. It also cancels `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL` and the `CLAUDE_CODE_USE_*` provider switches in `--settings`, so a key from your shell or from Claude Code's settings never reaches a provider. `graft --dry-run PROFILE` prints all of it.

## Configuration

The config file is `~/.config/graft/config.yaml`, or `$XDG_CONFIG_HOME/graft/config.yaml` when that variable is set; `--config FILE` or `GRAFT_CONFIG` picks another file. An unknown field is an error, and graft reports every problem of the file at once, each with its line.

### providers

| Field | Meaning |
| --- | --- |
| `url` | the base URL of the Anthropic Messages API, `http` or `https`, without a user name, password, query or fragment; Claude Code appends `/v1/messages` |
| `key` | where the key comes from: exactly one of `env: NAME`, `file: PATH` or `command: "CMD"` |

The key itself never goes into the config. A key file is trimmed of white space at the ends; the key must be one line of printable ASCII, so a byte order mark or a second line is an error. A command runs with `/bin/sh -c` in your terminal, so a password manager can ask for a password: `command: "pass show ollama"`. graft reads only the keys of the profile it starts, each once.

A provider name may contain lowercase letters, digits, `-` and `_`; `anthropic` and `graft` are taken.

### models

| Field | Default | Meaning |
| --- | --- | --- |
| `provider` | | a provider name |
| `id` | the model name | the model ID the provider expects |
| `context` | | the context window in tokens, 100000 or more |
| `label` | the model name | the `/model` row |
| `description` | `via <provider>` | its second line |
| `settings` | | Claude Code `modelSettings` for this model, such as `maxEffortLevel` |

A model name may contain letters, digits, `.`, `_` and `-`. It may not start with `claude` or be a Claude Code alias: `opus`, `sonnet`, `haiku`, `fable`, `default`, `best`, `opusplan`, `inherit`. Behind the router, Claude Code sees the model's name, so the same `id` at two providers can share a session; in the direct mode it sees the `id`.

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

A profile may not be named `list`, `check`, `help`, `version` or `ui`.

### Rules

- A config file is one YAML document. Anchors (`&x`, `*x`) work; merge keys (`<<`) do not.
- In `settings`, quote a date: unquoted, it is a YAML timestamp, which JSON lacks.
- `settings` may not hold `env`, `modelPicker` or `modelSettings`: graft builds them from `env`, `picker` and the settings of the models.
- `env` may not set the variables graft manages: the address, the keys, the models, the window and the provider switches. Its values are strings; write `""` for an empty one.
- `--settings` belongs to graft, in `args` and on the command line.
- In a direct profile, the external models need different `id`s, since Claude Code sees them.

## Commands

| Command | What it does |
| --- | --- |
| `graft PROFILE [ARGS...]` | starts Claude Code; ARGS go to `claude` |
| `graft --dry-run PROFILE [ARGS...]` | prints the environment, the routes, the command and the settings, with keys shown as fingerprints |
| `graft list` | lists the profiles and their modes |
| `graft check` | checks the config, the keys, `claude` and Claude Code's settings files |

## Security

- Keys never appear on the command line, in `--settings`, in the log or in graft's output: `--dry-run` and `check` show the first 12 hex digits of a key's SHA-256.
- The claude.ai token goes only to `https://api.anthropic.com`, an address in graft's code, not in the config. A provider gets its own key, never the claude.ai token.
- The router listens on `127.0.0.1` only and serves only the requests that carry the session secret, so neither another user's process nor a web page can spend your keys or your subscription through it.
- Claude Code passes its environment to the commands it runs, to hooks and to MCP servers. In the direct mode that includes the provider's key; behind the router, the keys stay in graft, and claude's environment holds only the session secret, which lets those processes use the router. `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB: "1"` in the profile's `env` strips the key and the secret from them, but it is Claude Code's hardening mode for CI, with side effects. On Linux, Claude Code 2.1.299 needs bubblewrap for it and runs commands in a sandbox where `ps` and `kill` do not see your other processes. At every start it also creates empty stand-ins for the files it guards, where they are missing: in your home, `~/.bash_profile` among them, after which a bash login shell no longer reads `~/.profile`; in the start directory, `package.json`, lock files and `.env` files, which it adds to `.git/info/exclude` once more each time.
- graft refuses to start a profile with external models when Claude Code's settings set `apiKeyHelper`, or an `env` with `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` or `ANTHROPIC_CUSTOM_HEADERS`, even an empty one: an empty value cancels graft's variable. graft reads `settings.json` in `CLAUDE_CONFIG_DIR` (taken from the profile's `env`, then from `defaults`, then from your environment) or in `~/.claude`, and `.claude/settings.json` and `.claude/settings.local.json` in the current directory. Such settings would send the provider's key elsewhere, or another key to the provider.
- Background sessions (`/background`, `claude --bg`) are off in profiles with external models, because a background session may lose the provider's address and key, or the router. Set `CLAUDE_CODE_DISABLE_AGENT_VIEW: "0"` in `env` at your own risk.

## Troubleshooting

| Message or symptom | What to do |
| --- | --- |
| `… Claude Code settings would override its address or key` | Remove the keys that graft names from those settings files, or start the profile from another directory if they are project files. |
| `provider x: env NAME is not set` | Export the variable in the shell that runs graft, or switch the key to `file` or `command`. |
| `key file … is readable by other users` | `chmod 600` the file. |
| `graft: model "x" is not configured in profile "p"` | The session asked for a model the profile does not route, and the profile has no subscription. Add the model to `picker`, or pick one from `/model`. |
| `graft: provider x unreachable: …` | The router cannot connect to the provider: check its `url`. The log names the request. |
| `Failed to authenticate. API Error: 401` after minutes of retries | A provider refused its key, and Claude Code retried the 401. The log shows `route=<provider> status=401`: fix that provider's key. |
| `graft: this router serves only its own Claude Code session` | A request came without the session secret, from something other than the session's `claude`. |
| Requests fail behind a proxy | The router uses the proxy of the shell that runs graft, not one from a profile's `env` or from Claude Code's settings files: export the proxy variables. Behind a proxy that inspects TLS, add its certificate to the system's store: the router does not read `NODE_EXTRA_CA_CERTS`. |
| Claude Code shows a 200k window or the wrong model | Run `graft --dry-run PROFILE` and check `CLAUDE_CODE_MAX_CONTEXT_TOKENS` and the aliases; a `--model` on the command line replaces the profile's model. |

## Limitations

- Anthropic does not support Claude Code on non-Claude models; it works, without support.
- graft speaks only the Anthropic Messages API. For an OpenAI-compatible provider, put CLIProxyAPI, LiteLLM or a similar gateway in between.
- Claude Code shows `unrecognized_model` and a model catalog warning for external models.
- Switching models in the middle of a conversation resets the prompt cache.
- The first turn of a Claude model after an external one costs a rejected request: Anthropic refuses the external model's thinking, and Claude Code sends the request again without it.
- `CLAUDE_CODE_AUTO_MODE_SERVER` is a temporary variable of Claude Code and may go away.
- If a profile's `settings` hold a value that Claude Code's schema rejects, Claude Code may drop all of `--settings`. `--dry-run` shows the JSON, `claude doctor` the errors.
- The check of Claude Code's settings files reads your user settings and `.claude/` in the current directory, not the settings of parent directories or managed settings.
- `/effort` saves the level in your Claude Code settings, under the model's name.
- The router takes its proxy from the environment graft runs in, not from a profile's `env` or Claude Code's settings files. Like Claude Code, it sends https requests through `HTTP_PROXY` when `HTTPS_PROXY` is not set. It trusts the system's certificates and ignores `NODE_EXTRA_CA_CERTS`.
- If graft is killed by a signal it cannot catch, such as `SIGKILL`, `claude` loses the router and its requests fail; go on with `graft PROFILE --resume`.

## License

[MIT](LICENSE)
