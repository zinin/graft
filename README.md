# graft

Graft other models onto Claude Code: run whole sessions on GPT, GLM, Kimi or DeepSeek, mix them with your Claude subscription in one `/model` picker, or let Opus lead while another model implements.

[What graft does](#what-graft-does) · [Install](#install) · [Quick start](#quick-start) · [Commands](#commands) · [Documentation](#documentation) · [Limits](#limits)

Claude Code works with any provider that speaks the Anthropic Messages API once `ANTHROPIC_BASE_URL` points at it. A working session takes a dozen environment variables, though, and all its requests go to that one address. graft keeps providers, models and profiles in one config file and starts Claude Code with a profile:

```sh
graft glm             # a whole session on GLM
graft glm --resume    # arguments after the profile go to claude
graft mix             # Opus on your subscription, with GPT and GLM in /model
graft ui              # edit the config in your browser
```

## What graft does

- **Whole sessions on another model.** A profile names the model, the effort and the Claude Code settings of a session, and Claude Code's aliases and subagents follow it.
- **Many models in one session.** When a profile mixes providers, or adds external models to a session on your Claude subscription, a local router sends each request where its model lives. See [How it works](docs/how-it-works.md).
- **Subagents on other models.** Claude can hand a task to any external model of the profile, so Opus can plan and review while GPT implements. See [Subagents](docs/subagents.md) and the example [GPT next to Claude](docs/gpt-next-to-claude.md).
- **A web editor.** `graft ui` offers presets of providers, writes key files, tests models and shows what a profile would start. See [Web UI](docs/web-ui.md).
- **Care with keys.** A key never goes into the config, the command line or the log, the claude.ai token goes only to Anthropic, and graft never writes to Claude Code's files. See [Security](docs/security.md).

## Install

graft runs on Linux and macOS with Claude Code 2.1.327 or later. To install the [latest release](https://github.com/zinin/graft/releases/latest) into `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/zinin/graft/master/install.sh | sh
```

[install.sh](install.sh) checks the release against its `SHA256SUMS`; run it again to update. For another release or directory, the archives and a build from source, see [Install](docs/install.md).

## Quick start

The quickest start is `graft ui`: in the page it serves, choose your provider, paste its key, test a model and save, then start the profile that the page names after the model, such as `graft glm-5.3-flash` for Ollama Cloud.

To set it up by hand:

1. Put your provider's key where graft can read it, for example in a file that only you can read:

   ```sh
   mkdir -p ~/.config/graft/keys
   rm -f ~/.config/graft/keys/ollama                # an old file would keep its permissions
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

3. Check the config with `graft check`, and see what graft would start with `graft --dry-run glm`.

4. Start the session: `graft glm`.

[config.example.yaml](config.example.yaml) has profiles that mix providers and the subscription, and [Configuration](docs/configuration.md) explains every field.

## Commands

| Command | What it does |
| --- | --- |
| `graft PROFILE [ARGS...]` | starts Claude Code; ARGS go to `claude` |
| `graft --dry-run PROFILE [ARGS...]` | prints the environment, the routes, the command, the settings and the agents, with keys shown as fingerprints |
| `graft list` | lists the profiles and their modes |
| `graft check` | checks the config, the keys, `claude` and Claude Code's settings files |
| `graft ui` | opens an editor of the config in your browser: see [Web UI](docs/web-ui.md) |

## Documentation

| Page | What it tells |
| --- | --- |
| [Install](docs/install.md) | another release or directory, the archives, a build from source |
| [How it works](docs/how-it-works.md) | the direct mode and the router: what Claude Code gets from graft |
| [Configuration](docs/configuration.md) | the fields of providers, models and profiles, and the rules |
| [Subagents](docs/subagents.md) | agents on external models: how to ask for one, its model and effort, how to hide it |
| [GPT next to Claude](docs/gpt-next-to-claude.md) | an example: a session on a Claude and a ChatGPT subscription |
| [Web UI](docs/web-ui.md) | `graft ui`: the presets, the keys, the tests and what a save does |
| [Security](docs/security.md) | where keys and tokens go, and who sees what |
| [Troubleshooting](docs/troubleshooting.md) | messages and symptoms, and what to do |
| [Limitations](docs/limitations.md) | what does not work, or works with a catch |

## Limits

Anthropic does not support Claude Code on non-Claude models; it works, without support. graft speaks only the Anthropic Messages API: for an OpenAI-compatible provider, put CLIProxyAPI, LiteLLM or a similar gateway in between. [Limitations](docs/limitations.md) has the rest.

## License

[MIT](LICENSE)
