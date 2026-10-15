# graft

Graft other models onto Claude Code: run whole sessions on GPT, GLM, Kimi or DeepSeek, or mix them with your Claude subscription in one `/model` picker.

Claude Code works with any provider that speaks the Anthropic Messages API once `ANTHROPIC_BASE_URL` points at it. A working session takes a dozen environment variables, though, usually kept in a shell function per model, and all the requests of a session go to that one address. graft keeps providers, models and profiles in one config file and starts Claude Code with the environment and settings of a profile. When a profile mixes providers, or adds external models to a session on your Claude subscription, a local router sends each request where its model lives:

```sh
graft glm             # a whole session on GLM
graft glm --resume    # arguments after the profile go to claude
graft mix             # Opus on your subscription, with GPT and GLM in /model
```

## Install

graft runs on Linux and macOS with Claude Code 2.1.288 or later. To install the latest release into `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/zinin/graft/master/install.sh | sh
```

[install.sh](install.sh) takes the build for your system from the [latest release](https://github.com/zinin/graft/releases/latest), checks it against the release's `SHA256SUMS` and installs it; run it again to update. `GRAFT_VERSION` picks another release and `GRAFT_INSTALL_DIR` another directory:

```sh
curl -fsSL https://raw.githubusercontent.com/zinin/graft/master/install.sh | GRAFT_VERSION=0.1.0 GRAFT_INSTALL_DIR=~/bin sh
```

A release also has the archives for a manual install, on x86-64 (`amd64`) and ARM (`arm64`). The macOS builds are not notarized: after a download in a browser, run `xattr -d com.apple.quarantine graft` before the first start.

With Go 1.26, `go install github.com/zinin/graft/cmd/graft@latest` builds graft from source. From a clone, `make install` puts the binary into `~/.local/bin`.

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
- `127.0.0.1` joins `NO_PROXY` and `no_proxy`, so that Claude Code reaches the router past any proxy;
- without the subscription, the same flags as in the direct mode: `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `CLAUDE_CODE_AUTO_MODE_SERVER=0`, `ENABLE_CLAUDEAI_MCP_SERVERS=false` and `CLAUDE_CODE_DISABLE_AGENT_VIEW=1`; with the subscription, only `CLAUDE_CODE_DISABLE_AGENT_VIEW=1`, since the auto mode, the claude.ai connectors and the background traffic work with your login. If you need the others, set them in the `env` of the profile or of `defaults`.

graft passes `SIGTERM` and `SIGHUP` on to `claude` and exits with its exit code; the router stops with `claude`. The router writes a line for each request to `~/.local/state/graft/graft.log`, or to `$XDG_STATE_HOME/graft/graft.log` when that variable is set: the model, where the request went, the status, how long it took. The log holds no headers, bodies, keys or secret.

In both modes, settings passed with `--settings` override your settings files for this session only. graft never writes to Claude Code's files or to your shell files, and only `graft ui` writes its config, when you save.

graft removes the variables it manages from the environment it inherits. It also cancels `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL` and the `CLAUDE_CODE_USE_*` provider switches in `--settings`, so a key from your shell or from Claude Code's settings never reaches a provider. `graft --dry-run PROFILE` prints all of it.

## Configuration

The config file is `~/.config/graft/config.yaml`, or `$XDG_CONFIG_HOME/graft/config.yaml` when that variable is set; `--config FILE` or `GRAFT_CONFIG` picks another file. An unknown field is an error, and graft reports every problem of the file at once, each with its line.

### providers

| Field | Meaning |
| --- | --- |
| `url` | the base URL of the Anthropic Messages API, `http` or `https`, without a user name, password, query or fragment; Claude Code appends `/v1/messages` |
| `key` | where the key comes from: exactly one of `env: NAME`, `file: PATH` or `command: "CMD"` |

The key itself never goes into the config. A key file is trimmed of white space at the ends; the key must be one line of printable ASCII, so a byte order mark or a second line is an error. A command runs with `/bin/sh -c` in your terminal, so a password manager can ask for a password: `command: "pass show ollama"`. graft reads only the keys of the profile it starts, each once. A key from `env` also stays in the environment of claude and of the commands, hooks and MCP servers it starts, in both modes; a key from a `file` or a `command` stays out of the session behind the router.

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
| `aliases` | see below | where `opus`, `sonnet`, `haiku` and `fable` point; without the subscription, `sonnet` also runs the auto mode classifier; `haiku` runs the background tasks |
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
- The `env` of Claude Code's settings files overrides your environment, so graft also passes the values that `env` gives to `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`, `CLAUDE_CODE_EFFORT_LEVEL`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`, `CLAUDE_CODE_AUTO_MODE_SERVER`, `ENABLE_CLAUDEAI_MCP_SERVERS` and `CLAUDE_CODE_DISABLE_AGENT_VIEW` in `--settings`, which your settings files do not override. The other variables of `env` stay out of `--settings`, which other users see in the process list, so they go only to the environment, where the `env` of Claude Code's settings files wins.
- `--settings` belongs to graft, in `args` and on the command line.
- In a direct profile, the external models need different `id`s, since Claude Code sees them.

## Commands

| Command | What it does |
| --- | --- |
| `graft PROFILE [ARGS...]` | starts Claude Code; ARGS go to `claude` |
| `graft --dry-run PROFILE [ARGS...]` | prints the environment, the routes, the command and the settings, with keys shown as fingerprints |
| `graft list` | lists the profiles and their modes |
| `graft check` | checks the config, the keys, `claude` and Claude Code's settings files |
| `graft ui` | opens an editor of the config in your browser: see [Web UI](#web-ui) |

## Web UI

`graft ui` edits the config in your browser. It edits the same file, so you can go on editing it by hand too.

```sh
graft ui
```

graft serves the page on `127.0.0.1` at a random port and prints its address; Ctrl+C stops it. In a graphical session (macOS, or Linux with a display) and not over SSH, graft opens the page itself; otherwise open the printed address yourself. The address opens the page in any browser on this machine until graft ui stops. Without a config, the page starts from an empty one and creates the file when you save. On a machine you reach over SSH, forward the port that graft prints, with the same number on both ends: `ssh -L 43117:127.0.0.1:43117 HOST`.

- Each change is checked by the same rules as the config part of `graft check`, and each problem shows at its field. A config with problems cannot be saved. The page neither creates key files nor reads them: write the key file yourself before you start a profile, as in the Quick start, and `graft check` tells whether graft can read it.
- Saving rewrites the whole file in one style: the comments stay, but blank lines inside sections, the alignment of comments and the spaces inside `{ }` go. The version before the last save is in `config.yaml.bak` next to the file; each save replaces it. When the config is a symbolic link, graft writes the file it points to, and the backup lies next to that file.
- The page looks at the file every 2 seconds. Without unsaved changes, it reloads the file; with them, it says that the file changed on disk and does not save over it. Reload then takes the file and drops the unsaved changes, and nothing is merged; Keep editing leaves them in the page, with Save off.
- Renaming a model or a provider renames it where profiles and models use it. Deleting an entry also deletes the comments above it.
- Changing a value that YAML aliases (`*x`) share gives each alias a copy of the old value, so that a change of one field never changes another. A settings field shows its value with the aliases expanded, and one that you change is saved without anchors.
- A file that is not one YAML document, a part of the wrong shape, such as a list where a name belongs, and an alias (`*x`) used as a name keep the forms closed: the page shows the problems with their lines in the file, and reloads it when you fix it in your editor. A field that graft does not know leaves the forms open, but only your editor can remove it, and the config cannot be saved until you do.

## Security

- Keys never appear on the command line, in `--settings`, in the log or in graft's output: `--dry-run` and `check` show the first 12 hex digits of a key's SHA-256.
- The claude.ai token goes only to `https://api.anthropic.com`, an address in graft's code, not in the config. A provider gets its own key, never the claude.ai token.
- The router listens on `127.0.0.1` only and serves only the requests that carry the session secret, so neither another user's process nor a web page can spend your keys or your subscription through it.
- Claude Code passes its environment to the commands it runs, to hooks and to MCP servers. In the direct mode the environment includes the provider's key. Behind the router, a key read from a `file` or a `command` stays in graft, and claude's environment holds only the session secret, which lets those processes use the router. A key from `env: NAME` stays in claude's environment under its own name in both modes, since claude inherits that variable from your shell like graft does; to keep a key out of the session, read it from a `file` or a `command`. `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB: "1"` in the profile's `env` strips `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_CUSTOM_HEADERS` (the direct mode's key and the session secret) from those processes, but do not count on it for the variable of `key: { env: NAME }`; it is Claude Code's hardening mode for CI, with side effects. On Linux, Claude Code 2.1.299 needs bubblewrap for it and runs commands in a sandbox where `ps` and `kill` do not see your other processes. At every start it also creates empty stand-ins for the files it guards, where they are missing: in your home, `~/.bash_profile` among them, after which a bash login shell no longer reads `~/.profile`; in the start directory, `package.json`, lock files and `.env` files, which it adds to `.git/info/exclude` once more each time.
- graft refuses to start a profile with external models when Claude Code's settings set `apiKeyHelper`, or an `env` with `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` or `ANTHROPIC_CUSTOM_HEADERS`, even an empty one: an empty value cancels graft's variable. graft reads `settings.json` in `CLAUDE_CONFIG_DIR` (taken from the profile's `env`, then from `defaults`, then from your environment) or in `~/.claude` (`.claude` under `HOME`, taken in the same order), and `.claude/settings.json` and `.claude/settings.local.json` in the current directory. graft also refuses `apiKeyHelper` in the `settings` of the profile or of `defaults`. Such settings would send the provider's key elsewhere, or another key to the provider.
- Background sessions (`/background`, `claude --bg`) are off in profiles with external models, because a background session may lose the provider's address and key, or the router. Set `CLAUDE_CODE_DISABLE_AGENT_VIEW: "0"` in `env` at your own risk.
- `graft ui` listens on `127.0.0.1` only and serves its API only with the token in the address it prints; it refuses requests for other host names and from other pages. On Linux, other users can read that address in the arguments of `xdg-open`, so it also refuses connections from their processes; your own processes and root are trusted. On macOS there is no such check: the kernel does not show other users' process arguments. Keep the address to yourself: a key source may be a command, so whoever writes the config runs commands as you.

## Troubleshooting

| Message or symptom | What to do |
| --- | --- |
| `… Claude Code settings would override its address or key` | Remove the keys that graft names from those settings files, or start the profile from another directory if they are project files. |
| `provider x: env NAME is not set` | Export the variable in the shell that runs graft, or switch the key to `file` or `command`. |
| `key file … is readable by other users` | `chmod 600` the file. |
| `graft: model "x" is not configured in profile "p"` | The session asked for a model the profile does not route, and the profile has no subscription. Add the model to `picker`, or pick one from `/model`. |
| `graft: provider x unreachable: …` | The router cannot connect to the provider: check its `url`. The log names the request. |
| `Failed to authenticate. API Error: 401` after minutes of retries | A provider refused its key, and Claude Code retried the 401. The log shows `route=<provider> status=401`: fix that provider's key. With `route=anthropic` instead, the 401 is about your claude.ai login. |
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
- In `/model` and `/effort`, Enter saves the choice in your Claude Code settings as the default for new sessions, and `s` makes it for this session only. A graft model saved as the default model breaks a plain `claude`, which then asks Anthropic for it. graft warns of such a model when a router session ends and in `graft check`. `/effort` saves the level under the model's name.
- The router takes its proxy from the environment graft runs in, not from a profile's `env` or Claude Code's settings files. Like Claude Code, it sends https requests through `HTTP_PROXY` when `HTTPS_PROXY` is not set. It trusts the system's certificates and ignores `NODE_EXTRA_CA_CERTS`.
- If graft is killed by a signal it cannot catch, such as `SIGKILL`, `claude` loses the router and its requests fail; go on with `graft PROFILE --resume`.
- The first save in `graft ui` rewrites the formatting of the whole file, and the page does not merge its changes with changes made to the file meanwhile: see [Web UI](#web-ui).
- `graft ui` refuses connections from other users' processes on Linux only: see [Security](#security).

## License

[MIT](LICENSE)
