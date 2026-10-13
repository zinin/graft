package plan

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

const testConfig = `
providers:
  cliproxy:
    url: http://127.0.0.1:8317
    key: { file: keys/cliproxy }
  ollama:
    url: https://ollama.com
    key: { env: OLLAMA_API_KEY }

models:
  sol:
    provider: cliproxy
    id: gpt-6.1-sol
    context: 400000
    label: GPT-6.1 Sol
    description: Codex via CLIProxyAPI
  glm-proxy: { provider: cliproxy, id: glm-5.3-flash, context: 524288 }
  glm:       { provider: ollama,   id: glm-5.3-flash, context: 1048576 }
  kimi:
    provider: ollama
    id: kimi-k3
    context: 1048576
    settings: { maxEffortLevel: high }

defaults:
  effort: max
  args: [--allow-dangerously-skip-permissions]

profiles:
  sol: { model: sol }
  glm-proxy:
    model: glm-proxy
    settings: { autoCompactEnabled: true }
  pair: { model: sol, picker: [glm-proxy] }
  kimi: { model: kimi }
  plain:
    model: opus
    subagents: claude-haiku-5-5
    settings: { autoCompactEnabled: true }
  tuned:
    model: glm
    effort: high
    env: { CLAUDE_CODE_DISABLE_AGENT_VIEW: "0", CLAUDE_CODE_EFFORT_LEVEL: max, FOO: bar }
  mix: { model: opus, picker: [sol, glm] }
  multi: { model: glm, picker: [sol] }
`

var shellEnv = []string{
	"PATH=/usr/bin",
	"ANTHROPIC_API_KEY=sk-ant-leak",
	"ANTHROPIC_BASE_URL=http://old.example",
	"CLAUDE_CODE_EFFORT_LEVEL=low",
}

func loadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(testConfig), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func build(t *testing.T, profile string, args ...string) *Plan {
	t.Helper()
	p, err := Build(Input{Config: loadConfig(t), Profile: profile, Args: args, Environ: shellEnv})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// compact removes the white space from a JSON text written for people.
func compact(t *testing.T, s string) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.Compact(&b, []byte(s)); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// settingsArg returns the value that follows --settings in args.
func settingsArg(t *testing.T, args []string) string {
	t.Helper()
	for i, a := range args {
		if a == "--settings" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("no --settings in %q", args)
	return ""
}

func TestDirectProfile(t *testing.T) {
	p := build(t, "glm-proxy", "--resume")

	if p.Mode != config.Direct || !reflect.DeepEqual(p.Providers, []string{"cliproxy"}) {
		t.Errorf("mode %v, providers %v", p.Mode, p.Providers)
	}
	if want := []string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_EFFORT_LEVEL"}; !reflect.DeepEqual(p.Unset, want) {
		t.Errorf("Unset = %q, want %q", p.Unset, want)
	}
	wantSet := []EnvVar{
		{Name: "ANTHROPIC_BASE_URL", Value: "http://127.0.0.1:8317"},
		{Name: "ANTHROPIC_AUTH_TOKEN", KeyOf: "cliproxy"},
		{Name: "ANTHROPIC_DEFAULT_OPUS_MODEL", Value: "glm-5.3-flash"},
		{Name: "ANTHROPIC_DEFAULT_SONNET_MODEL", Value: "glm-5.3-flash"},
		{Name: "ANTHROPIC_DEFAULT_HAIKU_MODEL", Value: "glm-5.3-flash"},
		{Name: "ANTHROPIC_DEFAULT_FABLE_MODEL", Value: "glm-5.3-flash"},
		{Name: "CLAUDE_CODE_SUBAGENT_MODEL", Value: "glm-5.3-flash"},
		{Name: "CLAUDE_CODE_MAX_CONTEXT_TOKENS", Value: "524288"},
		{Name: "CLAUDE_CODE_DISABLE_AGENT_VIEW", Value: "1"},
		{Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1"},
		{Name: "CLAUDE_CODE_AUTO_MODE_SERVER", Value: "0"},
		{Name: "ENABLE_CLAUDEAI_MCP_SERVERS", Value: "false"},
	}
	if !reflect.DeepEqual(p.Set, wantSet) {
		t.Errorf("Set =\n%+v\nwant\n%+v", p.Set, wantSet)
	}
	wantSettings := compact(t, `{
		"autoCompactEnabled": true,
		"env": {
			"ANTHROPIC_API_KEY": "",
			"ANTHROPIC_DEFAULT_FABLE_MODEL": "glm-5.3-flash",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL": "glm-5.3-flash",
			"ANTHROPIC_DEFAULT_OPUS_MODEL": "glm-5.3-flash",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "glm-5.3-flash",
			"ANTHROPIC_MODEL": "",
			"CLAUDE_CODE_AUTO_MODE_SERVER": "0",
			"CLAUDE_CODE_DISABLE_AGENT_VIEW": "1",
			"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
			"CLAUDE_CODE_EFFORT_LEVEL": "",
			"CLAUDE_CODE_MAX_CONTEXT_TOKENS": "524288",
			"CLAUDE_CODE_SUBAGENT_MODEL": "glm-5.3-flash",
			"CLAUDE_CODE_USE_ANTHROPIC_AWS": "",
			"CLAUDE_CODE_USE_BEDROCK": "",
			"CLAUDE_CODE_USE_FOUNDRY": "",
			"CLAUDE_CODE_USE_MANTLE": "",
			"CLAUDE_CODE_USE_VERTEX": "",
			"ENABLE_CLAUDEAI_MCP_SERVERS": "false"
		},
		"modelPicker": {
			"options": [{"description": "via cliproxy", "label": "glm-proxy", "model": "glm-5.3-flash"}],
			"replaceBuiltInOptions": true
		}
	}`)
	wantArgs := []string{
		"--model", "glm-5.3-flash", "--effort", "max", "--settings", wantSettings,
		"--allow-dangerously-skip-permissions", "--resume",
	}
	if !reflect.DeepEqual(p.Args, wantArgs) {
		t.Errorf("Args =\n%q\nwant\n%q", p.Args, wantArgs)
	}
}

func TestAddressAndKeyStayOutOfSettings(t *testing.T) {
	arg := settingsArg(t, build(t, "glm-proxy").Args)
	for _, s := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "127.0.0.1:8317"} {
		if strings.Contains(arg, s) {
			t.Errorf("--settings contains %q: %s", s, arg)
		}
	}
}

func TestCommandLineModelAndEffortWin(t *testing.T) {
	p := build(t, "sol", "--model", "other", "--effort=low")
	if got := p.Args[0]; got != "--settings" {
		t.Errorf("Args start with %q, want --settings: graft must not add --model or --effort", got)
	}
	if n := len(p.Args); p.Args[n-3] != "--model" || p.Args[n-1] != "--effort=low" {
		t.Errorf("Args = %q, want the command line at the end", p.Args)
	}
}

func TestOptionsAfterDoubleDashDoNotCount(t *testing.T) {
	p := build(t, "sol", "-p", "--", "--model")
	if p.Args[0] != "--model" || p.Args[1] != "gpt-6.1-sol" {
		t.Errorf("Args = %q, want graft's --model first", p.Args)
	}
}

func TestSettingsAfterDoubleDashIsText(t *testing.T) {
	p := build(t, "sol", "-p", "--", "--settings")
	if n := len(p.Args); p.Args[n-1] != "--settings" || p.Args[n-2] != "--" {
		t.Errorf("Args = %q, want the prompt text kept", p.Args)
	}
}

func TestSmallerWindowCompactsEarly(t *testing.T) {
	p := build(t, "pair")
	want := map[string]any{"gpt-6.1-sol": map[string]any{"autoCompactWindow": 400000}}
	if got := p.Settings["modelSettings"]; !reflect.DeepEqual(got, want) {
		t.Errorf("modelSettings = %v, want %v", got, want)
	}
	rows := p.Settings["modelPicker"].(map[string]any)["options"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["label"] != "GPT-6.1 Sol" || rows[1].(map[string]any)["model"] != "glm-5.3-flash" {
		t.Errorf("picker rows = %v", rows)
	}
	if got := envValue(p, "CLAUDE_CODE_MAX_CONTEXT_TOKENS"); got != "524288" {
		t.Errorf("CLAUDE_CODE_MAX_CONTEXT_TOKENS = %q, want the largest window", got)
	}
}

func TestModelSettingsReachClaude(t *testing.T) {
	want := map[string]any{"kimi-k3": map[string]any{"maxEffortLevel": "high"}}
	if got := build(t, "kimi").Settings["modelSettings"]; !reflect.DeepEqual(got, want) {
		t.Errorf("modelSettings = %v, want %v", got, want)
	}
}

func TestSubscriptionOnlyProfile(t *testing.T) {
	p := build(t, "plain")
	if want := []EnvVar{{Name: "CLAUDE_CODE_SUBAGENT_MODEL", Value: "claude-haiku-5-5"}}; !reflect.DeepEqual(p.Set, want) {
		t.Errorf("Set = %+v, want %+v", p.Set, want)
	}
	if want := []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY", "CLAUDE_CODE_EFFORT_LEVEL"}; !reflect.DeepEqual(p.Unset, want) {
		t.Errorf("Unset = %q, want %q", p.Unset, want)
	}
	if _, ok := p.Settings["modelPicker"]; ok {
		t.Error("a profile without external models must leave the /model rows alone")
	}
	if p.Args[0] != "--model" || p.Args[1] != "opus" {
		t.Errorf("Args = %q", p.Args)
	}
}

func TestProfileEnv(t *testing.T) {
	p := build(t, "tuned")
	if got := envValue(p, "CLAUDE_CODE_DISABLE_AGENT_VIEW"); got != "0" {
		t.Errorf("CLAUDE_CODE_DISABLE_AGENT_VIEW = %q, want the profile's 0", got)
	}
	n := len(p.Set)
	if p.Set[n-2] != (EnvVar{Name: "CLAUDE_CODE_EFFORT_LEVEL", Value: "max"}) || p.Set[n-1] != (EnvVar{Name: "FOO", Value: "bar"}) {
		t.Errorf("Set ends with %+v, want the profile's own variables in name order", p.Set[n-2:])
	}
	for _, name := range p.Unset {
		if name == "CLAUDE_CODE_EFFORT_LEVEL" {
			t.Error("the profile sets CLAUDE_CODE_EFFORT_LEVEL, so it must not be in Unset")
		}
	}
	env := p.Settings["env"].(map[string]any)
	if env["CLAUDE_CODE_DISABLE_AGENT_VIEW"] != "0" {
		t.Errorf("settings env CLAUDE_CODE_DISABLE_AGENT_VIEW = %v, want 0", env["CLAUDE_CODE_DISABLE_AGENT_VIEW"])
	}
	if _, ok := env["CLAUDE_CODE_EFFORT_LEVEL"]; ok {
		t.Error("settings env must not cancel CLAUDE_CODE_EFFORT_LEVEL that the profile sets")
	}
	if _, ok := env["FOO"]; ok {
		t.Error("the profile's own variables go only to the process environment")
	}
	if p.Args[2] != "--effort" || p.Args[3] != "high" {
		t.Errorf("Args = %q, want the profile's effort", p.Args)
	}
}

func TestBuildErrors(t *testing.T) {
	cfg := loadConfig(t)
	conflicting := []SettingsFile{{
		Path: "~/.claude/settings.json",
		Data: map[string]any{"apiKeyHelper": "get-key", "env": map[string]any{"ANTHROPIC_BASE_URL": "https://gateway.example"}},
	}}
	tests := []struct {
		name  string
		in    Input
		wants []string
	}{
		{"unknown profile", Input{Profile: "nope"}, []string{`no profile "nope"`}},
		{"settings on the command line", Input{Profile: "sol", Args: []string{"--settings", "x.json"}}, []string{"--settings is managed by graft"}},
		{"router", Input{Profile: "mix"}, []string{`profile "mix" needs the router, which is not implemented yet`}},
		{"router without subscription", Input{Profile: "multi"}, []string{`profile "multi" needs the router`}},
		{"conflicting settings", Input{Profile: "sol", ClaudeSettings: conflicting}, []string{
			`profile "sol" sends requests to an external provider`,
			"~/.claude/settings.json: apiKeyHelper",
			"~/.claude/settings.json: env.ANTHROPIC_BASE_URL",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Config = cfg
			_, err := Build(tt.in)
			if err == nil {
				t.Fatal("no error")
			}
			for _, want := range tt.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestConflictsDoNotStopSubscriptionProfile(t *testing.T) {
	conflicting := []SettingsFile{{Path: "x", Data: map[string]any{"apiKeyHelper": "get-key"}}}
	if _, err := Build(Input{Config: loadConfig(t), Profile: "plain", ClaudeSettings: conflicting}); err != nil {
		t.Fatal(err)
	}
}

func TestMerge(t *testing.T) {
	base := map[string]any{"a": 1, "m": map[string]any{"x": 1, "y": 2}, "l": []any{1}}
	over := map[string]any{"m": map[string]any{"y": 3}, "l": []any{2}, "b": true}
	got := merge(base, over)
	want := map[string]any{"a": 1, "b": true, "m": map[string]any{"x": 1, "y": 3}, "l": []any{2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merge = %v, want %v", got, want)
	}
	if base["m"].(map[string]any)["y"] != 2 {
		t.Error("merge changed base")
	}
}

func envValue(p *Plan, name string) string {
	for _, e := range p.Set {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}
