# Limitations

[graft](../README.md) › Limitations

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
- The first save in `graft ui` rewrites the formatting of the whole file, and the page does not merge its changes with changes made to the file meanwhile: see [Web UI](web-ui.md).
- `graft ui` refuses connections from other users' processes on Linux only: see [Security](security.md).
- The presets of `graft ui` go out of date as providers change their models; Test tells when a model ID no longer works.
