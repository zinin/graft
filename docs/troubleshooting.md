# Troubleshooting

[graft](../README.md) › Troubleshooting

| Message or symptom | What to do |
| --- | --- |
| `… Claude Code settings would override its address or key` | Remove the keys that graft names from those settings files, or start the profile from another directory if they are project files. |
| `provider x: env NAME is not set` | Export the variable in the shell that runs graft, or switch the key to `file` or `command`. |
| `claude in PATH is …, a batch file …` | On Windows, install Claude Code with its native installer, `irm https://claude.ai/install.ps1 \| iex`, and remove the npm one: `npm uninstall -g @anthropic-ai/claude-code`. |
| `key file … is readable by other users` | `chmod 600` the file (Linux and macOS). |
| `… the file is in UTF-16 …` | Save the key file as ASCII. Windows PowerShell 5.1 writes UTF-16 with `>` and `Out-File`; `Set-Content -Encoding ascii` writes ASCII. |
| `graft: model "x" is not configured in profile "p"` | The session asked for a model the profile does not route, and the profile has no subscription. Add the model to `picker`, or pick one from `/model`. |
| `graft: provider x unreachable: …` | The router cannot connect to the provider: check its `url`. The log names the request. |
| `Failed to authenticate. API Error: 401` after minutes of retries | A provider refused its key, and Claude Code retried the 401. The log shows `route=<provider> status=401`: fix that provider's key. With `route=anthropic` instead, the 401 is about your claude.ai login. |
| `graft: this router serves only its own Claude Code session` | A request came without the session secret, from something other than the session's `claude`. |
| Requests fail behind a proxy | The router uses the proxy of the shell that runs graft, not one from a profile's `env` or from Claude Code's settings files: export the proxy variables. Behind a proxy that inspects TLS, add its certificate to the system's store: the router does not read `NODE_EXTRA_CA_CERTS`. |
| `Wrong model: this agent runs on …` from a subagent | Claude passed `model` in the call, which replaced the agent's own. It should call the agent again without it; if it does not, ask it to. With no `model` in the call, `CLAUDE_CODE_SUBAGENT_MODEL_FORCE` or the `availableModels` of Claude Code's settings replaced it. |
| Claude does not see graft's agents | `graft --dry-run PROFILE` shows which agents the profile has: an `--agents` of yours that is a file or empty replaces them. Claude Code before 2.1.327 loses them once plugins load: update it. |
| `--agents is disabled by your organization's managed settings` | The organization's `disableSideloadFlags` forbids graft's agents: start without them, with `args: [--agents, ""]` in the profile or in `defaults`. |
| Claude Code shows a 200k window or the wrong model | Run `graft --dry-run PROFILE` and check `CLAUDE_CODE_MAX_CONTEXT_TOKENS` and the aliases; a `--model` on the command line replaces the profile's model. |
