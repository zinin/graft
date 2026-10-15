package edit

import (
	"errors"
	"fmt"
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
		{"x: &k glm\nmodels:\n  *k : {provider: p, context: 100000}\n", "line 3: models: a key must not be an alias (*k)"},
		{"profiles:\n  x:\n    settings: &s\n      a: *s\n", "line 3: profiles.x.settings: recursive alias *s"},
		{"models:\n  m:\n    settings: {a: &a {b: &b [*a]}}\n", "line 3: models.m.settings: recursive alias *a"},
		{"a: 1\n---\nb: 2\n", "line 2: must be one YAML document"},
		{"providers: [\n", "line 2: did not find expected node content"},
		{"providers:\n  x:\n    url: [https://a\n    key: {file: k}\n", "line 3: did not find expected ',' or ']'"},
		{"profiles:\n  a: { model: opus }\n---\nproviders:\n  x:\n    url: [https://a\n    key: {file: k}\n",
			"line 6: did not find expected ',' or ']'"},
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

// aliasBomb returns a flow mapping of levels lists: the first holds ten
// scalars and each next one ten aliases of the one before, so the last one
// expands to 10^levels nodes from a few hundred bytes of text.
func aliasBomb(levels int) string {
	ten := func(item string) string { return strings.TrimSuffix(strings.Repeat(item+", ", 10), ", ") }
	parts := []string{"l0: &l0 [" + ten("x") + "]"}
	for i := 1; i < levels; i++ {
		parts = append(parts, fmt.Sprintf("l%d: &l%d [%s]", i, i, ten(fmt.Sprintf("*l%d", i-1))))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// TestReadExpandsAliasesInSettings: the text of a settings value stands on
// its own, with a copy of what each alias in it points at and no anchor,
// because the browser cannot see the rest of the file. The copy keeps the
// comments and the style of what it copies.
func TestReadExpandsAliasesInSettings(t *testing.T) {
	d, err := Read([]byte(`defaults:
  settings: &common
    # shared
    permissions: &p {allow: [Bash]}
profiles:
  x:
    model: opus
    settings:
      permissions: *p
      extra: 1
  y:
    model: opus
    settings: *common
  z:
    model: opus
    settings:
      a: &c 1 # base
      b: *c # same
      c: *c
`))
	if err != nil {
		t.Fatal(err)
	}
	shared := "# shared\npermissions: {allow: [Bash]}\n"
	if d.Defaults.Settings != shared || d.Profiles[1].Settings != shared {
		t.Errorf("defaults %q, y %q, want %q", d.Defaults.Settings, d.Profiles[1].Settings, shared)
	}
	if want := "permissions: {allow: [Bash]}\nextra: 1\n"; d.Profiles[0].Settings != want {
		t.Errorf("x = %q, want %q", d.Profiles[0].Settings, want)
	}
	// A comment on an alias is the alias's, not a copy of the one on its anchor.
	if want := "a: 1 # base\nb: 1 # same\nc: 1\n"; d.Profiles[2].Settings != want {
		t.Errorf("z = %q, want %q", d.Profiles[2].Settings, want)
	}
}

// TestReadShowsAnEmptyValueInAFlowCollectionAsNull: the settings text of
// {a: } says null, the value in the file, not an empty string in quotes. In
// block style the empty value is written as nothing, as it is in the file.
func TestReadShowsAnEmptyValueInAFlowCollectionAsNull(t *testing.T) {
	d, err := Read([]byte(`profiles:
  x: {model: opus, settings: {a: , b: &z , c: *z}}
  y:
    model: opus
    settings:
      a:
`))
	if err != nil {
		t.Fatal(err)
	}
	if want := "{a: null, b: null, c: null}\n"; d.Profiles[0].Settings != want {
		t.Errorf("x = %q, want %q", d.Profiles[0].Settings, want)
	}
	if want := "a:\n"; d.Profiles[1].Settings != want {
		t.Errorf("y = %q, want %q", d.Profiles[1].Settings, want)
	}
}

// TestReadAliasBomb: a few lines of aliases of aliases are refused as a
// problem of the settings field, instead of filling the memory.
func TestReadAliasBomb(t *testing.T) {
	file := func(levels int) []byte {
		return []byte("profiles:\n  x:\n    model: opus\n    settings: " + aliasBomb(levels) + "\n")
	}
	_, err := Read(file(12))
	var se *ShapeError
	if !errors.As(err, &se) || !strings.Contains(err.Error(), "profiles.x.settings: the aliases expand to more than") {
		t.Errorf("Read = %v, want a ShapeError about profiles.x.settings", err)
	}
	// An expansion far below the limit is no problem.
	if _, err := Read(file(3)); err != nil {
		t.Errorf("Read of a small expansion: %v", err)
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

// TestSameLeavesItsArguments: comparing drafts must not change them.
func TestSameLeavesItsArguments(t *testing.T) {
	a := Draft{Profiles: []Profile{{Name: "x"}}}
	b := Draft{Profiles: []Profile{{Name: "x"}}}
	Same(a, b)
	if a.Profiles[0].Picker != nil || a.Profiles[0].Aliases != nil || b.Profiles[0].Picker != nil {
		t.Errorf("Same changed its arguments: %+v, %+v", a.Profiles[0], b.Profiles[0])
	}
}
