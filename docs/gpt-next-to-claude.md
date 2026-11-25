# Example: GPT next to Claude

[graft](../README.md) › Example: GPT next to Claude

[What it is good for](#what-it-is-good-for) · [Set it up](#set-it-up) · [Work with it](#work-with-it) · [What it costs](#what-it-costs)

A ChatGPT subscription comes without an API key. [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) signs in to it as Codex does and serves its models over the Anthropic Messages API. So one session can run on two subscriptions: Claude's, with your claude.ai login, and ChatGPT's. The profile `gpt-plus` is such a session.

## What it is good for

- **Opus leads, GPT implements.** Opus plans, hands the tasks of the plan to GPT agents and reviews their work. Planning and review spend your Claude limits, and implementation spends the Codex quota. The work of an agent stays out of Opus's context: it gets the report.
- **A second opinion.** A model of another vendor reviews a design or a diff, in the same session and with the same tools.
- **A way on.** When one provider is overloaded or a limit is used up, `/model` moves the conversation to the other.
- **One setup.** All models share your CLAUDE.md, permissions, skills and MCP servers. The auto mode and the claude.ai connectors work with your Claude login.

## Set it up

1. Install CLIProxyAPI as its [documentation](https://help.router-for.me/) says. In its config, set `server.host` to `127.0.0.1`, so that only this machine reaches it, and put a random string of your own into `access.api-keys`: the key that its clients must send. Then sign it in to Codex:

   ```sh
   cli-proxy-api --codex-login           # in a browser on this machine
   cli-proxy-api --codex-device-login    # with a code, in a browser anywhere
   ```

   Do not give it the login of the Codex CLI, `~/.codex/auth.json`: programs that share a login sign each other out. For the same reason, sign in on each machine anew.

2. Put the client key into `~/.config/graft/keys/cliproxy`, as in [Quick start](../README.md#quick-start), and write the config:

   ```yaml
   providers:
     cliproxy:
       url: http://127.0.0.1:8317
       key: { file: ~/.config/graft/keys/cliproxy }

   models:
     sol:
       provider: cliproxy
       id: gpt-6.2-sol
       context: 872000
       label: GPT-6.2 Sol
       description: Codex via CLIProxyAPI
     astra:
       provider: cliproxy
       id: gpt-6.2-astra
       context: 872000
       label: GPT-6.2 Astra
       description: Codex via CLIProxyAPI
     luna:
       provider: cliproxy
       id: gpt-6-luna
       context: 872000
       label: GPT-6 Luna
       description: Codex via CLIProxyAPI

   profiles:
     gpt-plus:
       model: sol
       picker: [astra, luna]
       subscription: true
       effort: max
       settings: { autoCompactEnabled: true }
   ```

   872000 is the largest window that Codex names for these models. Claude reads `description` when it chooses among the models, so say there what each is good for.

3. Check it with `graft check`. In `graft ui`, Test sends a model one short request.

## Work with it

```sh
graft gpt-plus                 # starts on Sol
graft gpt-plus --model opus    # starts on Opus
```

In `/model`, `s` switches the model for this session; Enter also makes it the default for new ones. Ask for an agent by the name of its model:

> Plan this change with me. Then give each task of the plan to astra, review its diff yourself and run the tests.

> Ask sol for a second opinion on this design.

To have Opus lead without asking each time, put the request into CLAUDE.md, or into the profile's `args` as in `lead` in [config.example.yaml](../config.example.yaml). `/tasks` shows which model works on what, and so does the log:

```
2026-11-25T10:15:00Z session=3f9a1c POST /v1/messages model=astra route=cliproxy status=200 dur=13.2s bytes=6208
```

## What it costs

- An agent runs at the profile's `effort`, here `max`, and its [progress lines](subagents.md) spend the Codex quota as well.
- A switch of the main model starts the prompt cache anew, and the first turn of a Claude model after GPT costs one rejected request: see [Limitations](limitations.md).
- Both providers get to see what their models work on: see [Security](security.md).
- A new Codex model works once CLIProxyAPI's catalog has it; until then, it answers `unknown provider for model`.
