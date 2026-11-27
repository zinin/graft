# Configuration

[graft](../README.md) › Configuration

[providers](#providers) · [models](#models) · [profiles](#profiles) · [rules](#rules)

The config file is `~/.config/graft/config.yaml`, or `$XDG_CONFIG_HOME/graft/config.yaml` when that variable is set; `--config FILE` or `GRAFT_CONFIG` picks another file. An unknown field is an error, and graft reports every problem of the file at once, each with its line.

## providers

| Field | Meaning |
| --- | --- |
| `url` | the base URL of the Anthropic Messages API, `http` or `https`, without a user name, password, query or fragment; Claude Code appends `/v1/messages` |
| `key` | where the key comes from: exactly one of `env: NAME`, `file: PATH` or `command: "CMD"` |

The key itself never goes into the config. A key file is trimmed of white space at the ends; the key must be one line of printable ASCII, so a byte order mark or a second line is an error. A command runs with `/bin/sh -c` in your terminal, so a password manager can ask for a password: `command: "pass show ollama"`. graft reads only the keys of the profile it starts, each once. A key from `env` also stays in the environment of claude and of the commands, hooks and MCP servers it starts, in both modes; a key from a `file` or a `command` stays out of the session behind the router.

A provider name may contain lowercase letters, digits, `-` and `_`; `anthropic` and `graft` are taken.

## models

| Field | Default | Meaning |
| --- | --- | --- |
| `provider` | | a provider name |
| `id` | the model name | the model ID the provider expects |
| `context` | | the context window in tokens, 100000 or more |
| `label` | the model name | the `/model` row |
| `description` | `via <provider>` | its second line, which Claude also reads in its list of [subagents](subagents.md) |
| `settings` | | Claude Code `modelSettings` for this model, such as `maxEffortLevel` |

A model name may contain letters, digits, `.`, `_` and `-`. It may not start with `claude` or be a Claude Code alias: `opus`, `sonnet`, `haiku`, `fable`, `default`, `best`, `opusplan`, `inherit`. Behind the router, Claude Code sees the model's name, so the same `id` at two providers can share a session; in the direct mode it sees the `id`.

## profiles

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

## Rules

- A config file is one YAML document. Anchors (`&x`, `*x`) work; merge keys (`<<`) do not.
- In `settings`, quote a date: unquoted, it is a YAML timestamp, which JSON lacks.
- `settings` may not hold `env`, `modelPicker` or `modelSettings`: graft builds them from `env`, `picker` and the settings of the models.
- `env` may not set the variables graft manages, in any case: the address, the keys, the models, the window and the provider switches. On Windows a name is one variable in any case, so one `env` may not name a variable twice in different cases. The values are strings; write `""` for an empty one.
- The `env` of Claude Code's settings files overrides your environment, so graft also passes the values that `env` gives to `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`, `CLAUDE_CODE_EFFORT_LEVEL`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`, `CLAUDE_CODE_AUTO_MODE_SERVER`, `ENABLE_CLAUDEAI_MCP_SERVERS` and `CLAUDE_CODE_DISABLE_AGENT_VIEW` in `--settings`, which your settings files do not override. The other variables of `env` stay out of `--settings`, which other users see in the process list, so they go only to the environment, where the `env` of Claude Code's settings files wins.
- `--settings` belongs to graft, in `args` and on the command line.
- In a direct profile, the external models need different `id`s, since Claude Code sees them.
