package edit

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func readExample(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReadExample(t *testing.T) {
	d, err := Read(readExample(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Providers) != 3 || len(d.Models) != 5 || len(d.Profiles) != 5 {
		t.Fatalf("counts %d %d %d", len(d.Providers), len(d.Models), len(d.Profiles))
	}
	if got, want := d.Providers[0], (Provider{Was: "cliproxy", Name: "cliproxy", URL: "http://127.0.0.1:8317",
		Key: Key{Kind: "file", Value: "~/.config/graft/keys/cliproxy"}}); got != want {
		t.Errorf("providers[0] = %+v, want %+v", got, want)
	}
	if got := d.Providers[2].Key; got != (Key{Kind: "command", Value: "pass show mimo/api-key"}) {
		t.Errorf("mimo key = %+v", got)
	}
	kimi := d.Models[3]
	if kimi.Name != "kimi" || kimi.ID != "kimi-k3" || kimi.Context != "1048576" || kimi.Label != "" ||
		!strings.Contains(kimi.Settings, "maxEffortLevel: high") {
		t.Errorf("kimi = %+v", kimi)
	}
	if want := (Defaults{Effort: "max", Env: []Pair{}, Args: []string{"--allow-dangerously-skip-permissions"}}); !reflect.DeepEqual(d.Defaults, want) {
		t.Errorf("defaults = %+v", d.Defaults)
	}
	mix := d.Profiles[2]
	if mix.Model != "opus" || mix.Subagents != "glm" || !reflect.DeepEqual(mix.Picker, []string{"sol", "glm", "kimi"}) ||
		mix.Subscription != "" || mix.Aliases["haiku"] != "" || len(mix.Aliases) != 4 {
		t.Errorf("mix = %+v", mix)
	}
	if multi := d.Profiles[4]; multi.Aliases["haiku"] != "glm-proxy" {
		t.Errorf("multi aliases = %v", multi.Aliases)
	}
}

func TestReadEmpty(t *testing.T) {
	for _, data := range []string{"", "# only a comment\n", "~\n"} {
		d, err := Read([]byte(data))
		if err != nil {
			t.Fatalf("Read(%q): %v", data, err)
		}
		if d.Providers == nil || d.Models == nil || d.Profiles == nil || d.Defaults.Env == nil || d.Defaults.Args == nil {
			t.Errorf("Read(%q) has nil lists: %+v", data, d)
		}
	}
}

func TestReadKeepsKeysOutOfTheDraft(t *testing.T) {
	d, err := Read([]byte("providers:\n  p:\n    url: https://x\n    key: sk-secret\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Providers[0].Key != (Key{}) {
		t.Errorf("key = %+v, want none", d.Providers[0].Key)
	}
}

func TestReadTakesTheFirstKeySource(t *testing.T) {
	d, err := Read([]byte("providers:\n  p:\n    key: { command: c, env: E }\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Providers[0].Key; got != (Key{Kind: "env", Value: "E"}) {
		t.Errorf("key = %+v", got)
	}
}

func TestReadFollowsAliasesAndNulls(t *testing.T) {
	d, err := Read([]byte(`
models:
  a: &m { provider: p, context: 100000 }
  b: *m
  c:
profiles:
  x:
    model: a
    env: { FOO: "", BAR: "1" }
`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Models[1].Provider != "p" || d.Models[1].Context != "100000" || d.Models[2].Name != "c" {
		t.Errorf("models = %+v", d.Models)
	}
	if want := []Pair{{"FOO", ""}, {"BAR", "1"}}; !reflect.DeepEqual(d.Profiles[0].Env, want) {
		t.Errorf("env = %v, want %v", d.Profiles[0].Env, want)
	}
}

func TestReadIgnoresUnknownFields(t *testing.T) {
	d, err := Read([]byte("models:\n  m: { provider: p, contxt: 5 }\nextra: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Models[0].Context != "" {
		t.Errorf("model = %+v", d.Models[0])
	}
}

func TestReadShapeErrors(t *testing.T) {
	tests := []struct{ data, want string }{
		{"providers: [a]\n", "line 1: providers: must be a mapping"},
		{"models:\n  m: text\n", "line 2: models.m: must be a mapping"},
		{"profiles:\n  p: { picker: glm }\n", "line 2: profiles.p.picker: must be a list"},
		{"profiles:\n  p: { picker: [a, {b: c}] }\n", "line 2: profiles.p.picker[1]: must be a single value"},
		{"profiles:\n  p: { model: [a] }\n", "line 2: profiles.p.model: must be a single value"},
		{"profiles:\n  p: { env: { A: [1] } }\n", "line 2: profiles.p.env.A: must be a single value"},
		{"profiles:\n  p: {}\n  p: {}\n", "line 3: profiles.p: duplicate key"},
		{"a: 1\n---\nb: 2\n", "line 2: must be one YAML document"},
		{"providers: [\n", "yaml: line"},
		{"- a\n", "line 1: must be a mapping"},
	}
	for _, tt := range tests {
		_, err := Read([]byte(tt.data))
		var se *ShapeError
		if !errors.As(err, &se) {
			t.Errorf("Read(%q) = %v, want a ShapeError", tt.data, err)
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Read(%q) = %q, want %q", tt.data, err, tt.want)
		}
	}
}

func TestSame(t *testing.T) {
	a := Draft{Profiles: []Profile{{Was: "x", Name: "x", Settings: "a: 1\nb: [2]\n"}}}
	b := Draft{Profiles: []Profile{{Name: "x", Settings: "{b: [2], a: 1}"}}}
	if !Same(a, b) {
		t.Error("Was or the form of settings counted")
	}
	b.Profiles[0].Settings = "a: 2\n"
	if Same(a, b) {
		t.Error("different settings counted as the same")
	}
	b.Profiles[0].Settings = a.Profiles[0].Settings
	b.Profiles[0].Picker = []string{"glm"}
	if Same(a, b) {
		t.Error("different pickers counted as the same")
	}
}
