package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const example = `
providers:
  cliproxy:
    url: http://127.0.0.1:8317
    key: { file: keys/cliproxy }
  ollama:
    url: https://ollama.com
    key: { env: OLLAMA_API_KEY }
  mimo:
    url: https://api.xiaomimimo.com/anthropic
    key: { command: "pass show mimo/api-key" }

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
  mimo-pro: { provider: mimo, id: mimo-v2.6-pro, context: 1048576 }

defaults:
  effort: max
  args: [--allow-dangerously-skip-permissions]

profiles:
  sol: { model: sol }
  glm-proxy:
    model: glm-proxy
    settings: { autoCompactEnabled: true }
  mix:
    model: opus
    picker: [sol, glm, kimi]
    subagents: glm
  sol-plus:
    model: sol
    subscription: true
  multi:
    model: glm
    picker: [sol, kimi]
    aliases: { haiku: glm-proxy }
`

// parse parses src as the file config.yaml in a fresh directory.
func parse(t *testing.T, src string) (*Config, error) {
	t.Helper()
	return Parse([]byte(src), filepath.Join(t.TempDir(), "config.yaml"))
}

func TestParseExample(t *testing.T) {
	cfg, err := parse(t, example)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.ProviderOrder, []string{"cliproxy", "ollama", "mimo"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ProviderOrder = %v, want %v", got, want)
	}
	if got, want := cfg.ProfileOrder, []string{"sol", "glm-proxy", "mix", "sol-plus", "multi"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ProfileOrder = %v, want %v", got, want)
	}
	if got, want := cfg.Providers["cliproxy"].Key.File, filepath.Join(filepath.Dir(cfg.Path), "keys", "cliproxy"); got != want {
		t.Errorf("relative key file = %q, want %q", got, want)
	}
	if got := cfg.Providers["mimo"].Key; got != (KeySource{Command: "pass show mimo/api-key"}) {
		t.Errorf("mimo key = %+v", got)
	}
	glm := cfg.Models["glm"]
	if glm.ID != "glm-5.3-flash" || glm.Label != "glm" || glm.Description != "via ollama" || glm.Context != 1048576 {
		t.Errorf("defaults of model glm = %+v", glm)
	}
	if got := cfg.Models["kimi"].Settings; !reflect.DeepEqual(got, map[string]any{"maxEffortLevel": "high"}) {
		t.Errorf("kimi settings = %v", got)
	}
	if cfg.Defaults.Effort != "max" || !reflect.DeepEqual(cfg.Defaults.Args, []string{"--allow-dangerously-skip-permissions"}) {
		t.Errorf("defaults = %+v", cfg.Defaults)
	}

	mix := cfg.Profiles["mix"]
	if mix.Model != (Ref{Claude: "opus"}) || !mix.Subscription || *mix.Subagents != (Ref{Model: "glm"}) {
		t.Errorf("mix = %+v", mix)
	}
	if solPlus := cfg.Profiles["sol-plus"]; !solPlus.Subscription || solPlus.Model != (Ref{Model: "sol"}) {
		t.Errorf("sol-plus = %+v", solPlus)
	}
	if sol := cfg.Profiles["sol"]; sol.Subscription {
		t.Error("sol: subscription must default to false for a graft model")
	}
	if got := cfg.Profiles["multi"].Aliases["haiku"]; got != (Ref{Model: "glm-proxy"}) {
		t.Errorf("multi haiku alias = %+v", got)
	}
}

func TestModes(t *testing.T) {
	cfg, err := parse(t, example)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		profile   string
		mode      Mode
		models    []string
		providers []string
	}{
		{"sol", Direct, []string{"sol"}, []string{"cliproxy"}},
		{"glm-proxy", Direct, []string{"glm-proxy"}, []string{"cliproxy"}},
		{"mix", Router, []string{"glm", "sol", "kimi"}, []string{"ollama", "cliproxy"}},
		{"sol-plus", Router, []string{"sol"}, []string{"cliproxy"}},
		{"multi", Router, []string{"glm", "glm-proxy", "sol", "kimi"}, []string{"ollama", "cliproxy"}},
	}
	for _, tt := range tests {
		p := cfg.Profiles[tt.profile]
		if got := cfg.ModeOf(p); got != tt.mode {
			t.Errorf("%s: mode %v, want %v", tt.profile, got, tt.mode)
		}
		var names []string
		for _, m := range cfg.ExternalModels(p) {
			names = append(names, m.Name)
		}
		if !reflect.DeepEqual(names, tt.models) {
			t.Errorf("%s: external models %v, want %v", tt.profile, names, tt.models)
		}
		if got := cfg.ProvidersOf(p); !reflect.DeepEqual(got, tt.providers) {
			t.Errorf("%s: providers %v, want %v", tt.profile, got, tt.providers)
		}
	}
}

func TestSubscriptionOnlyProfileIsDirect(t *testing.T) {
	cfg, err := parse(t, "profiles:\n  plain: { model: opus, subagents: claude-haiku-5-5 }\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ModeOf(cfg.Profiles["plain"]); got != Direct {
		t.Errorf("mode = %v, want direct", got)
	}
}

func TestHomeInKeyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := parse(t, `
providers:
  p: { url: "https://example.com", key: { file: ~/keys/p } }
models:
  m: { provider: p, context: 200000 }
profiles:
  m: { model: m }
`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Providers["p"].Key.File, filepath.Join(home, "keys", "p"); got != want {
		t.Errorf("key file = %q, want %q", got, want)
	}
}

func TestEnvAndSettingsValues(t *testing.T) {
	cfg, err := parse(t, `
defaults:
  env: { A: 1, B: true, C: text, D: "" }
  settings:
    autoCompactEnabled: true
    permissions: { defaultMode: auto, allow: [Read] }
    since: "2026-10-14"
profiles:
  plain: { model: opus }
`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Defaults.Env, map[string]string{"A": "1", "B": "true", "C": "text", "D": ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("env = %v, want %v", got, want)
	}
	data, err := json.Marshal(cfg.Defaults.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"autoCompactEnabled":true,"permissions":{"allow":["Read"],"defaultMode":"auto"},"since":"2026-10-14"}`; got != want {
		t.Errorf("settings JSON = %s, want %s", got, want)
	}
}

// base is a valid config that the error cases extend.
const base = `
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
  q: { url: "https://example.org", key: { env: Q_KEY } }
models:
  m: { provider: p, context: 200000 }
  n: { provider: q, context: 200000 }
`

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // a substring of the error: line, path and message
	}{
		{"syntax", "profiles: [", "yaml:"},
		{"empty file", "", "profiles: at least one profile is required"},
		{"comments only", "# nothing yet\n", "profiles: at least one profile is required"},
		{"second document", base + "profiles:\n  a: { model: m }\n---\nprofiles:\n  b: { model: m }\n",
			":10: the config must be one YAML document"},
		{"root not a mapping", "- a\n", ":1: must be a mapping"},
		{"unknown top field", "extra: 1\nprofiles:\n  a: { model: opus }\n", ":1: extra: unknown field"},
		{"no profiles", "providers: {}\n", "profiles: at least one profile is required"},
		{"duplicate key", "profiles:\n  a: { model: opus }\n  a: { model: opus }\n", ":3: profiles.a: duplicate key"},

		{"provider name", "providers:\n  Bad: { url: \"https://x.y\", key: { env: K } }\nprofiles:\n  a: { model: opus }\n",
			":2: providers.Bad: a provider name may contain"},
		{"reserved provider", "providers:\n  anthropic: { url: \"https://x.y\", key: { env: K } }\nprofiles:\n  a: { model: opus }\n",
			`:2: providers.anthropic: "anthropic" is reserved`},
		{"url required", "providers:\n  p: { key: { env: K } }\nprofiles:\n  a: { model: opus }\n", ":2: providers.p.url: required"},
		{"url relative", "providers:\n  p: { url: example.com, key: { env: K } }\nprofiles:\n  a: { model: opus }\n",
			":2: providers.p.url: must be an absolute http or https URL"},
		{"url with credentials", "providers:\n  p: { url: \"https://user:pw@x.y\", key: { env: K } }\nprofiles:\n  a: { model: opus }\n",
			":2: providers.p.url: must not contain a user name or password"},
		{"url with a key as user", "providers:\n  p: { url: \"https://sk-123@x.y/api\", key: { env: K } }\nprofiles:\n  a: { model: opus }\n",
			":2: providers.p.url: must not contain a user name or password"},
		{"url with query", "providers:\n  p: { url: \"https://x.y/api?v=1\", key: { env: K } }\nprofiles:\n  a: { model: opus }\n",
			":2: providers.p.url: must not have a query or fragment"},
		{"url with empty query", "providers:\n  p: { url: \"https://x.y/api?\", key: { env: K } }\nprofiles:\n  a: { model: opus }\n",
			":2: providers.p.url: must not have a query or fragment"},
		{"url with fragment", "providers:\n  p: { url: \"https://x.y/api#v1\", key: { env: K } }\nprofiles:\n  a: { model: opus }\n",
			":2: providers.p.url: must not have a query or fragment"},
		{"literal key", "providers:\n  p: { url: \"https://x.y\", key: sk-123 }\nprofiles:\n  a: { model: opus }\n",
			"providers.p.key: say where the key comes from"},
		{"two key sources", "providers:\n  p: { url: \"https://x.y\", key: { env: K, file: f } }\nprofiles:\n  a: { model: opus }\n",
			"providers.p.key: set exactly one of env, file, command"},
		{"no key source", "providers:\n  p: { url: \"https://x.y\", key: {} }\nprofiles:\n  a: { model: opus }\n",
			"providers.p.key: set exactly one of env, file, command"},
		{"bad env name", "providers:\n  p: { url: \"https://x.y\", key: { env: 1BAD } }\nprofiles:\n  a: { model: opus }\n",
			"providers.p.key.env: not a valid variable name"},
		{"unknown key field", "providers:\n  p: { url: \"https://x.y\", key: { value: x } }\nprofiles:\n  a: { model: opus }\n",
			"providers.p.key.value: unknown field"},

		{"model named claude", base + "  claude-x: { provider: p, context: 200000 }\nprofiles:\n  a: { model: m }\n",
			`models.claude-x: a model name must not start with "claude"`},
		{"model named alias", base + "  Opus: { provider: p, context: 200000 }\nprofiles:\n  a: { model: m }\n",
			`models.Opus: "Opus" is a Claude Code alias`},
		{"model provider required", base + "  x: { context: 200000 }\nprofiles:\n  a: { model: m }\n", "models.x.provider: required"},
		{"model provider unknown", base + "  x: { provider: nope, context: 200000 }\nprofiles:\n  a: { model: m }\n",
			`models.x.provider: no provider "nope"`},
		{"context required", base + "  x: { provider: p }\nprofiles:\n  a: { model: m }\n", "models.x.context: required"},
		{"context small", base + "  x: { provider: p, context: 99999 }\nprofiles:\n  a: { model: m }\n",
			"models.x.context: must be at least 100000"},
		{"context not integer", base + "  x: { provider: p, context: big }\nprofiles:\n  a: { model: m }\n",
			"models.x.context: must be an integer"},
		{"id with space", base + "  x: { provider: p, id: \"a b\", context: 200000 }\nprofiles:\n  a: { model: m }\n",
			"models.x.id: must be non-empty and contain no white space"},

		{"reserved profile", base + "profiles:\n  list: { model: m }\n", `profiles.list: "list" is a graft command`},
		{"profile model required", base + "profiles:\n  a: { effort: max }\n", "profiles.a.model: required"},
		{"unknown model", base + "profiles:\n  a: { model: nope }\n", `profiles.a.model: unknown model "nope"`},
		{"claude model without subscription", base + "profiles:\n  a: { model: opus, subscription: false }\n",
			`profiles.a.model: "opus" is a Claude model: it needs subscription: true`},
		{"claude subagents without subscription", base + "profiles:\n  a: { model: m, subagents: claude-haiku-5-5 }\n",
			`profiles.a.subagents: "claude-haiku-5-5" is a Claude model: it needs subscription: true`},
		{"alias to alias", base + "profiles:\n  a: { model: opus, aliases: { opus: sonnet } }\n",
			`profiles.a.aliases.opus: unknown model "sonnet"`},
		{"unknown alias", base + "profiles:\n  a: { model: m, aliases: { gpt: m } }\n", "profiles.a.aliases.gpt: unknown alias"},
		{"picker unknown", base + "profiles:\n  a: { model: m, picker: [m, x] }\n",
			`profiles.a.picker[1]: unknown model "x": picker lists the models of this config`},
		{"subscription not bool", base + "profiles:\n  a: { model: m, subscription: yes please }\n",
			"profiles.a.subscription: must be true or false"},
		{"effort", base + "profiles:\n  a: { model: m, effort: huge }\n",
			"profiles.a.effort: must be one of low, medium, high, xhigh, max"},
		{"settings env", base + "profiles:\n  a: { model: m, settings: { env: { A: b } } }\n",
			"profiles.a.settings.env: graft builds env itself: use env"},
		{"settings modelSettings", "defaults:\n  settings: { modelSettings: {} }\n" + base + "profiles:\n  a: { model: m }\n",
			"defaults.settings.modelSettings: graft builds modelSettings itself"},
		{"settings infinity", base + "profiles:\n  a: { model: m, settings: { x: .inf } }\n",
			":9: profiles.a.settings.x: .inf is not a JSON number"},
		{"settings negative infinity", "defaults:\n  settings: { a: -.inf }\n" + base + "profiles:\n  a: { model: m }\n",
			":2: defaults.settings.a: -.inf is not a JSON number"},
		{"settings NaN in a list", base + "  x: { provider: p, context: 200000, settings: { l: [1, .NaN] } }\nprofiles:\n  a: { model: m }\n",
			":8: models.x.settings.l[1]: .NaN is not a JSON number"},
		{"settings key not a string", base + "profiles:\n  a: { model: m, settings: { 1: x } }\n",
			":9: profiles.a.settings: keys must be strings"},
		{"settings date", base + "profiles:\n  a: { model: m, settings: { x: 2026-10-14 } }\n",
			":9: profiles.a.settings.x: 2026-10-14 is a YAML timestamp, which JSON lacks: quote it"},
		{"settings time in a list", base + "  x: { provider: p, context: 200000, settings: { l: [1, 2026-10-14T21:04:05Z] } }\nprofiles:\n  a: { model: m }\n",
			":8: models.x.settings.l[1]: 2026-10-14T21:04:05Z is a YAML timestamp, which JSON lacks: quote it"},
		{"owned env", base + "profiles:\n  a: { model: m, env: { ANTHROPIC_BASE_URL: x } }\n",
			"profiles.a.env.ANTHROPIC_BASE_URL: graft manages this variable itself"},
		{"bad env var name", base + "profiles:\n  a: { model: m, env: { \"A-B\": x } }\n",
			"profiles.a.env.A-B: not a valid variable name"},
		{"env tilde", base + "profiles:\n  a: { model: m, env: { FOO: ~ } }\n",
			`:9: profiles.a.env.FOO: must be a string: write "" for an empty value`},
		{"env null", "defaults:\n  env: { FOO: null }\n" + base + "profiles:\n  a: { model: m }\n",
			`:2: defaults.env.FOO: must be a string: write "" for an empty value`},
		{"env without a value", base + "profiles:\n  a:\n    model: m\n    env:\n      FOO:\n",
			`profiles.a.env.FOO: must be a string: write "" for an empty value`},
		{"settings flag", base + "profiles:\n  a: { model: m, args: [--verbose, --settings, x.json] }\n",
			"profiles.a.args[1]: --settings is managed by graft"},
		{"settings flag with value", "defaults:\n  args: [\"--settings={}\"]\n" + base + "profiles:\n  a: { model: m }\n",
			"defaults.args[0]: --settings is managed by graft"},
		{"defaults unknown field", "defaults:\n  model: m\n" + base + "profiles:\n  a: { model: m }\n",
			"defaults.model: unknown field"},
		{"direct profile with one id twice", `
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
models:
  a: { provider: p, id: same, context: 200000 }
  b: { provider: p, id: same, context: 300000 }
profiles:
  two: { model: a, picker: [b] }
`, `profiles.two: models "a" and "b" share the id "same"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(t, tt.src)
			if err == nil {
				t.Fatalf("no error, want %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error:\n%v\nwant it to contain %q", err, tt.want)
			}
		})
	}
}

func TestAllErrorsAtOnce(t *testing.T) {
	_, err := parse(t, base+"profiles:\n  a: { model: nope }\n  b: { model: m, effort: huge }\n")
	var errs Errors
	if !errors.As(err, &errs) {
		t.Fatalf("error %v is not Errors", err)
	}
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2:\n%v", len(errs), err)
	}
	// base begins with a newline, so profile a is on line 9 and b on line 10.
	if errs[0].Line != 9 || errs[1].Line != 10 {
		t.Errorf("lines %d and %d, want 9 and 10", errs[0].Line, errs[1].Line)
	}
}

// TestOneMistakeOneError: a provider or model with an error is left out of
// the config, but the references to it are not errors of their own.
func TestOneMistakeOneError(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // a substring of each error, in order
	}{
		{"provider without a scheme", `
providers:
  p: { url: example.com, key: { env: K } }
  q: { url: "https://x.y", key: { env: K } }
models:
  m: { provider: p, context: 200000 }
  m2: { provider: p, context: 200000 }
  n: { provider: q, context: 200000 }
profiles:
  a: { model: m }
  b: { model: n, picker: [m2], aliases: { haiku: m }, subagents: m2 }
`, []string{":3: providers.p.url: must be an absolute http or https URL"}},
		{"reserved provider name", `
providers:
  graft: { url: "https://x.y", key: { env: K } }
models:
  m: { provider: graft, context: 200000 }
profiles:
  a: { model: m }
`, []string{`:3: providers.graft: "graft" is reserved`}},
		{"provider with a bad name", `
providers:
  Bad: { url: "https://x.y", key: { env: K } }
models:
  m: { provider: Bad, context: 200000 }
profiles:
  a: { model: m }
`, []string{":3: providers.Bad: a provider name may contain"}},
		{"providers not a mapping", `
providers:
  - p
models:
  m: { provider: p, context: 200000 }
profiles:
  a: { model: m }
`, []string{":3: providers: must be a mapping"}},
		{"model without context", `
providers:
  p: { url: "https://x.y", key: { env: K } }
models:
  m: { provider: p }
  n: { provider: p, context: 200000 }
profiles:
  a: { model: m }
  b: { model: n, picker: [m] }
`, []string{":5: models.m.context: required"}},
		{"model with an unknown provider", `
providers:
  p: { url: "https://x.y", key: { env: K } }
models:
  m: { provider: pp, context: 200000 }
profiles:
  a: { model: m }
`, []string{`:5: models.m.provider: no provider "pp"`}},
		{"model with a bad name", `
providers:
  p: { url: "https://x.y", key: { env: K } }
models:
  "glm 5": { provider: p, context: 200000 }
profiles:
  a: { model: "glm 5" }
`, []string{":5: models.glm 5: a model name may contain"}},
		{"model named like an alias", `
providers:
  p: { url: "https://x.y", key: { env: K } }
models:
  Opus: { provider: p, context: 200000 }
profiles:
  a: { model: Opus }
`, []string{`:5: models.Opus: "Opus" is a Claude Code alias`}},
		{"model named like claude", `
providers:
  p: { url: "https://x.y", key: { env: K } }
models:
  claudette: { provider: p, context: 200000 }
profiles:
  a: { model: claudette }
`, []string{`:5: models.claudette: a model name must not start with "claude"`}},
		{"models not a mapping", `
models: [m]
profiles:
  a: { model: m }
`, []string{":2: models: must be a mapping"}},
		{"an unknown model next to a broken one", `
providers:
  p: { url: "https://x.y", key: { env: K } }
models:
  m: { provider: p }
profiles:
  a: { model: m, picker: [x] }
`, []string{":5: models.m.context: required", `:7: profiles.a.picker[0]: unknown model "x"`}},
		{"model not a string", base + "profiles:\n  a: { model: [m] }\n",
			[]string{":9: profiles.a.model: must be a string"}},
		{"subagents not a string", base + "profiles:\n  a: { model: m, subagents: { x: 1 } }\n",
			[]string{":9: profiles.a.subagents: must be a string"}},
		{"alias not a string", base + "profiles:\n  a: { model: m, aliases: { haiku: [m] } }\n",
			[]string{":9: profiles.a.aliases.haiku: must be a string"}},
		{"provider not a string", base + "  x: { provider: [p], context: 200000 }\nprofiles:\n  a: { model: m }\n",
			[]string{":8: models.x.provider: must be a string"}},
		{"repeated key in settings", base + "profiles:\n  a:\n    model: m\n    settings:\n      permissions:\n" +
			"        allow: [Read]\n        allow: [Bash]\n",
			[]string{":14: profiles.a.settings.permissions.allow: duplicate key"}},
		{"infinity behind an alias used twice", base + "profiles:\n  a: { model: m, settings: { a: &x { v: .inf }, b: *x } }\n",
			[]string{":9: profiles.a.settings.a.v: .inf is not a JSON number"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(t, tt.src)
			var errs Errors
			if !errors.As(err, &errs) {
				t.Fatalf("error %v is not Errors", err)
			}
			if len(errs) != len(tt.want) {
				t.Fatalf("got %d errors, want %d:\n%v", len(errs), len(tt.want), err)
			}
			for i, want := range tt.want {
				if !strings.Contains(errs[i].Error(), want) {
					t.Errorf("error %d is %q, want it to contain %q", i, errs[i].Error(), want)
				}
			}
		})
	}
}

func TestDocumentStartMarker(t *testing.T) {
	if _, err := parse(t, "---\n"+base+"profiles:\n  a: { model: m }\n"); err != nil {
		t.Fatal(err)
	}
}

func TestSameIDOnTwoProvidersIsFineWithRouter(t *testing.T) {
	_, err := parse(t, `
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
  q: { url: "https://example.org", key: { env: Q_KEY } }
models:
  a: { provider: p, id: same, context: 200000 }
  b: { provider: q, id: same, context: 300000 }
profiles:
  two: { model: a, picker: [b] }
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnchorsAndMergeKeys(t *testing.T) {
	cfg, err := parse(t, `
profiles:
  a: { model: opus, settings: &compact { autoCompactEnabled: true } }
  b: { model: opus, settings: *compact }
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Profiles["b"].Settings; !reflect.DeepEqual(got, map[string]any{"autoCompactEnabled": true}) {
		t.Errorf("settings through an alias = %v", got)
	}
	_, err = parse(t, "profiles:\n  c:\n    <<: { model: opus }\n")
	if err == nil || !strings.Contains(err.Error(), "profiles.c.<<: unknown field") {
		t.Errorf("merge key: error %v, want it refused as an unknown field", err)
	}
}

// TestMergeKeysInSettingsAreRefused: yaml.v3 applies merge keys when it
// decodes settings, which would let env, modelPicker or modelSettings past
// the checks.
func TestMergeKeysInSettingsAreRefused(t *testing.T) {
	tests := map[string]string{
		"profile":  "profiles:\n  a:\n    model: opus\n    settings: { <<: { env: { ANTHROPIC_API_KEY: sk-x } } }\n",
		"defaults": "defaults:\n  settings:\n    <<: { modelPicker: { options: [] } }\nprofiles:\n  a: { model: opus }\n",
		"model":    base + "  x: { provider: p, context: 200000, settings: { inner: { <<: { a: 1 } } } }\nprofiles:\n  a: { model: m }\n",
	}
	for name, src := range tests {
		if _, err := parse(t, src); err == nil || !strings.Contains(err.Error(), "merge keys (<<) are not supported") {
			t.Errorf("%s: error %v, want the merge key refused", name, err)
		}
	}
}

func TestExampleFile(t *testing.T) {
	if _, err := Load(filepath.Join("..", "..", "config.example.yaml")); err != nil {
		t.Fatal(err)
	}
}

// TestReadmeExample parses the first yaml block of README.md.
func TestReadmeExample(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	var block []string
	indent, inside := "", false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !inside && trimmed == "```yaml" {
			indent, inside = line[:strings.Index(line, "```")], true
			continue
		}
		if inside {
			if trimmed == "```" {
				break
			}
			block = append(block, strings.TrimPrefix(line, indent))
		}
	}
	if len(block) == 0 {
		t.Fatal("README.md has no yaml block")
	}
	if _, err := parse(t, strings.Join(block, "\n")); err != nil {
		t.Fatal(err)
	}
}
