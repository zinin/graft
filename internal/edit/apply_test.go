package edit

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

// change reads data, lets edit change the draft, applies it and checks that
// the result reads back as the changed draft and passes config.Parse when
// valid is set. It returns the new file.
func change(t *testing.T, data string, valid bool, edit func(d *Draft)) string {
	t.Helper()
	d, err := Read([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	edit(&d)
	out, err := Apply([]byte(data), d)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	back, err := Read(out)
	if err != nil {
		t.Fatalf("Read of the result: %v\n%s", err, out)
	}
	if !Same(back, d) {
		t.Errorf("the result reads back as\n%+v\nwant\n%+v\nfile:\n%s", back, d, out)
	}
	if _, err := config.Parse(out, "/home/u/.config/graft/config.yaml"); valid && err != nil {
		t.Errorf("config.Parse: %v\nfile:\n%s", err, out)
	}
	return string(out)
}

func TestApplyWithoutChangesKeepsComments(t *testing.T) {
	data := string(readExample(t))
	out := change(t, data, true, func(*Draft) {})
	for _, comment := range []string{
		"# graft config: copy to ~/.config/graft/config.yaml and edit.",
		"cliproxy: # CLIProxyAPI on this machine",
		"label: GPT-6.1 Sol # the /model row; defaults to the name",
		"  # The whole session on one external model: graft sol",
		"  # GPT as the main model; agents with model: opus and the auto mode",
	} {
		if !strings.Contains(out, comment) {
			t.Errorf("lost %q:\n%s", comment, out)
		}
	}
	for _, section := range []string{"\n\n# Models: the name", "\n\n# Defaults for every profile", "\n\nprofiles:"} {
		if !strings.Contains(out, section) {
			t.Errorf("no blank line before %q:\n%s", section, out)
		}
	}
}

func TestApplyScalars(t *testing.T) {
	data := `models:
  sol:
    provider: cliproxy
    context: 400000
    label: GPT Sol # the /model row
    description: old
profiles:
  sol: { model: sol, effort: max }
`
	out := change(t, data, false, func(d *Draft) {
		d.Models[0].Label = "true"
		d.Models[0].Context = "abc"
		d.Models[0].Description = ""
		d.Models[0].ID = "gpt-6.1-sol"
		d.Profiles[0].Subscription = "true"
	})
	for _, want := range []string{
		`    label: "true" # the /model row`,
		"    context: abc\n",
		"    provider: cliproxy\n    id: gpt-6.1-sol\n    context:",
		"sol: {model: sol, subscription: true, effort: max}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "description") {
		t.Errorf("description was not removed:\n%s", out)
	}
}

// TestApplyContextWithALeadingZero: YAML reads 0400000 as an octal number,
// so a context typed that way stays text and config.Parse reports it,
// instead of the file saving a window other than the one the form shows.
func TestApplyContextWithALeadingZero(t *testing.T) {
	data := `providers:
  p: {url: "https://p.example", key: {env: P_KEY}}
models:
  m:
    provider: p
    context: 400000
profiles:
  x: {model: m}
`
	tests := []struct{ context, want, problem string }{
		{"0400000", "    context: \"0400000\"\n", "models.m.context: must be an integer"},
		{"0", "    context: 0\n", "models.m.context: must be at least 100000"},
		{"100000", "    context: 100000\n", ""},
	}
	for _, tt := range tests {
		out := change(t, data, tt.problem == "", func(d *Draft) { d.Models[0].Context = tt.context })
		if !strings.Contains(out, tt.want) {
			t.Errorf("context %s: want %q in:\n%s", tt.context, tt.want, out)
		}
		_, err := config.Parse([]byte(out), "/home/u/.config/graft/config.yaml")
		if tt.problem == "" && err != nil || tt.problem != "" && (err == nil || !strings.Contains(err.Error(), tt.problem)) {
			t.Errorf("context %s: config.Parse = %v, want %q", tt.context, err, tt.problem)
		}
	}
}

func TestApplyEntries(t *testing.T) {
	data := `providers:
  # local proxy
  cliproxy: # on this machine
    url: http://127.0.0.1:8317
    key: { file: keys/cliproxy }
models:
  glm: { provider: cliproxy, context: 524288 }
profiles:
  # the first one
  a: { model: glm }
  # the second one
  b: { model: glm }
`
	out := change(t, data, true, func(d *Draft) {
		d.Providers[0].Name = "proxy"
		d.Models[0].Provider = "proxy"
		d.Profiles = []Profile{d.Profiles[1], {Name: "c", Model: "glm", Effort: "high"}}
	})
	for _, want := range []string{
		"  # local proxy\n  proxy: # on this machine\n",
		"  # the second one\n  b: {model: glm}\n  c:\n    model: glm\n    effort: high\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "the first one") {
		t.Errorf("profile a and its comment were not removed:\n%s", out)
	}
}

func TestApplySwapsNames(t *testing.T) {
	data := "profiles:\n  a: { model: opus }\n  b: { model: sonnet }\n"
	out := change(t, data, true, func(d *Draft) {
		d.Profiles[0].Name, d.Profiles[1].Name = "b", "a"
	})
	if want := "profiles:\n  b: {model: opus}\n  a: {model: sonnet}\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyFromNothing(t *testing.T) {
	out := change(t, "", true, func(d *Draft) {
		d.Profiles = append(d.Profiles, Profile{Name: "glm", Model: "glm"})
		d.Models = append(d.Models, Model{Name: "glm", Provider: "ollama", Context: "1048576"})
		d.Providers = append(d.Providers, Provider{Name: "ollama", URL: "https://ollama.com",
			Key: Key{Kind: "file", Value: "~/.config/graft/keys/ollama"}})
	})
	want := `providers:
  ollama:
    url: https://ollama.com
    key: {file: ~/.config/graft/keys/ollama}

models:
  glm:
    provider: ollama
    context: 1048576

profiles:
  glm:
    model: glm
`
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyRemovesEmptiedSections(t *testing.T) {
	data := "defaults:\n  effort: max\nmodels:\n  m: { provider: p, context: 100000 }\nprofiles:\n  x: { model: opus }\n"
	out := change(t, data, true, func(d *Draft) {
		d.Defaults.Effort = ""
		d.Models = nil
	})
	if want := "profiles:\n  x: {model: opus}\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyLists(t *testing.T) {
	data := `defaults:
  args:
    - --one
profiles:
  x: { model: opus, picker: [a, b] }
models:
  a: { provider: p, context: 100000 }
  b: { provider: p, context: 100000 }
  c: { provider: p, context: 100000 }
`
	out := change(t, data, false, func(d *Draft) {
		d.Defaults.Args = append(d.Defaults.Args, "--two")
		d.Profiles[0].Picker = []string{"c", "a"}
	})
	for _, want := range []string{"  args:\n    - --one\n    - --two\n", "x: {model: opus, picker: [c, a]}"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	out = change(t, data, false, func(d *Draft) { d.Profiles[0].Picker = nil })
	if strings.Contains(out, "picker") {
		t.Errorf("picker was not removed:\n%s", out)
	}
}

func TestApplyEnv(t *testing.T) {
	data := `profiles:
  x:
    model: opus
    env:
      KEEP: "1" # stays
      CHANGE: a
      DROP: b
`
	out := change(t, data, true, func(d *Draft) {
		d.Profiles[0].Env = []Pair{{"KEEP", "1"}, {"CHANGE", ""}, {"NEW", "0"}}
	})
	if want := "    env:\n      KEEP: \"1\" # stays\n      CHANGE: \"\"\n      NEW: \"0\"\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Env = nil })
	if strings.Contains(out, "env") {
		t.Errorf("env was not removed:\n%s", out)
	}
}

func TestApplyAliases(t *testing.T) {
	data := `models:
  glm: { provider: p, context: 100000 }
profiles:
  x:
    model: glm
    picker: [glm]
    effort: max
  y:
    model: glm
    aliases: { haiku: glm, opux: glm }
`
	out := change(t, data, false, func(d *Draft) {
		d.Profiles[0].Aliases["haiku"] = "glm"
		d.Profiles[1].Aliases["haiku"] = ""
	})
	for _, want := range []string{"    picker: [glm]\n    aliases: {haiku: glm}\n    effort: max\n", "aliases: {opux: glm}"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	data = "profiles:\n  y: { model: opus, aliases: { haiku: claude-haiku-5-5 } }\n"
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Aliases["haiku"] = "" })
	if want := "profiles:\n  y: {model: opus}\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyKey(t *testing.T) {
	data := `providers:
  a:
    url: https://a
    key: { file: keys/a }
  b:
    url: https://b
    key:
      env: B_KEY
`
	out := change(t, data, false, func(d *Draft) {
		d.Providers[0].Key = Key{Kind: "env", Value: "A_KEY"}
		d.Providers[1].Key = Key{Kind: "command", Value: "pass show b"}
	})
	for _, want := range []string{"    key: {env: A_KEY}\n", "    key:\n      command: pass show b\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}

func TestApplySettings(t *testing.T) {
	data := `defaults:
  settings: &common
    autoCompactEnabled: true
profiles:
  x:
    model: opus
    settings: *common
`
	out := change(t, data, true, func(d *Draft) {
		d.Profiles[0].Settings = "# mine\nautoCompactEnabled: false\n"
	})
	if want := "    settings:\n      # mine\n      autoCompactEnabled: false\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	if !strings.Contains(out, "  settings: &common\n    autoCompactEnabled: true\n") {
		t.Errorf("defaults lost its settings or anchor:\n%s", out)
	}
	out = change(t, data, true, func(d *Draft) { d.Defaults.Settings = `{"autoCompactEnabled": false}` })
	if want := "defaults:\n  settings: {\"autoCompactEnabled\": false}\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	if want := "    settings:\n      autoCompactEnabled: true\n"; !strings.Contains(out, want) || strings.Contains(out, "common") {
		t.Errorf("the alias did not get a copy of the old value:\n%s", out)
	}
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Settings = "  # nothing\n" })
	if strings.Contains(out, "*common") {
		t.Errorf("settings were not removed:\n%s", out)
	}
}

// TestApplySettingsWithANestedAlias: a settings text shows the value an
// alias in it stands for, so changing the field of the anchor, or the text
// itself, keeps the round trip.
func TestApplySettingsWithANestedAlias(t *testing.T) {
	data := `defaults:
  settings:
    permissions: &p {allow: [Bash]}
profiles:
  x:
    model: opus
    settings:
      permissions: *p
`
	out := change(t, data, true, func(*Draft) {})
	if !strings.Contains(out, "permissions: &p {allow: [Bash]}") || !strings.Contains(out, "permissions: *p") {
		t.Errorf("a save without changes lost the anchor or the alias:\n%s", out)
	}
	out = change(t, data, true, func(d *Draft) { d.Defaults.Settings = "other: 1\n" })
	if want := "    settings:\n      permissions: {allow: [Bash]}\n"; !strings.Contains(out, want) {
		t.Errorf("the alias did not get a copy of the old value:\n%s", out)
	}
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Settings += "extra: 1\n" })
	if want := "    settings:\n      permissions: {allow: [Bash]}\n      extra: 1\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	if !strings.Contains(out, "permissions: &p {allow: [Bash]}") {
		t.Errorf("defaults lost its anchor:\n%s", out)
	}
}

// TestApplySettingsWithAnAnchoredID: the same for an anchor on a field of a
// model, which a settings text uses.
func TestApplySettingsWithAnAnchoredID(t *testing.T) {
	data := `providers:
  p: {url: "https://p.example", key: {env: P_KEY}}
models:
  glm: {provider: p, id: &i glm-5, context: 100000}
profiles:
  x:
    model: glm
    settings: {model: *i}
`
	edits := []func(d *Draft){
		func(d *Draft) { d.Models[0].ID = "glm-6" },
		func(d *Draft) { d.Models, d.Profiles[0].Model = nil, "opus" },
	}
	for _, edit := range edits {
		out := change(t, data, true, edit)
		if want := "    settings: {model: glm-5}\n"; !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}

// TestApplySettingsTextWithAnAnchor: an anchor and an alias typed into a
// settings text do not reach the file, so the anchor cannot shadow one that
// the file has, and the alias is a copy.
func TestApplySettingsTextWithAnAnchor(t *testing.T) {
	out := change(t, "profiles:\n  x: {model: opus}\n", true, func(d *Draft) { d.Profiles[0].Settings = "a: &c 1\nb: *c\n" })
	if want := "x: {model: opus, settings: {a: 1, b: 1}}\n"; !strings.Contains(out, want) || strings.ContainsAny(out, "&*") {
		t.Errorf("want %q and no anchor or alias in:\n%s", want, out)
	}
	data := `defaults:
  settings: &c
    a: 1
profiles:
  x: {model: opus}
  y:
    model: opus
    settings: *c
`
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Settings = "&c\nb: 2" })
	if strings.Count(out, "&c") != 1 || !strings.Contains(out, "    settings: *c\n") {
		t.Errorf("the anchor of the text shadows the one of the file:\n%s", out)
	}
}

func TestApplyKeepsAChangeOutOfAliases(t *testing.T) {
	data := `models:
  a: { provider: p, id: &i glm-5, context: 100000 }
  b: { provider: p, id: *i, context: 100000 }
`
	out := change(t, data, false, func(d *Draft) { d.Models[0].ID = "glm-6" })
	if !strings.Contains(out, "a: {provider: p, id: glm-6, context: 100000}") || !strings.Contains(out, "b: {provider: p, id: glm-5, context: 100000}") {
		t.Errorf("got:\n%s", out)
	}
	out = change(t, data, false, func(d *Draft) { d.Models = d.Models[1:] })
	if want := "models:\n  b: {provider: p, id: glm-5, context: 100000}\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyCopiesAliasedEntries(t *testing.T) {
	data := `models:
  a: &m { provider: p, context: 100000 }
  b: *m
`
	out := change(t, data, false, func(d *Draft) { d.Models[1].Context = "200000" })
	if !strings.Contains(out, "a: &m {provider: p, context: 100000}") || !strings.Contains(out, "b: {provider: p, context: 200000}") {
		t.Errorf("got:\n%s", out)
	}
}

// anchoredKey is a config whose model name is an anchor that another model
// uses as its label.
const anchoredKey = `providers:
  p: {url: "https://p.example", key: {env: P_KEY}}
models:
  &n glm: {provider: p, context: 100000}
  other: {provider: p, context: 100000, label: *n}
profiles:
  x: {model: other}
`

// TestApplyRenamesAnAnchoredKey: the label of another model is the old
// name of the key, not a name that follows the key.
func TestApplyRenamesAnAnchoredKey(t *testing.T) {
	out := change(t, anchoredKey, true, func(d *Draft) { d.Models[0].Name = "glm2" })
	for _, want := range []string{"  glm2: {provider: p, context: 100000}\n", "  other: {provider: p, context: 100000, label: glm}\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}

// TestApplyRemovesAnAnchoredKey: the alias of a removed key gets its name,
// instead of pointing at an anchor that is gone.
func TestApplyRemovesAnAnchoredKey(t *testing.T) {
	out := change(t, anchoredKey, true, func(d *Draft) { d.Models = d.Models[1:] })
	if want := "models:\n  other: {provider: p, context: 100000, label: glm}\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
}

// TestApplyFillsAnAnchoredNull: a null entry that another entry aliases
// becomes a mapping, and the other entry stays null.
func TestApplyFillsAnAnchoredNull(t *testing.T) {
	out := change(t, "profiles:\n  a: &x\n  b: *x\n", false, func(d *Draft) { d.Profiles[0].Model = "opus" })
	if want := "profiles:\n  a:\n    model: opus\n  b:\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

// TestApplyKeepsUnknownFields: the forms do not show a field graft does
// not know, and a save leaves it for the user to fix in the editor.
func TestApplyKeepsUnknownFields(t *testing.T) {
	data := "models:\n  m: { provider: p, context: 100000, contxt: 5 }\nextra: 1\n"
	out := change(t, data, false, func(d *Draft) { d.Models[0].Context = "200000" })
	if !strings.Contains(out, "m: {provider: p, context: 200000, contxt: 5}") || !strings.Contains(out, "extra: 1") {
		t.Errorf("got:\n%s", out)
	}
}

func TestApplyErrors(t *testing.T) {
	data := "profiles:\n  a: { model: opus }\n"
	d, _ := Read([]byte(data))
	tests := []struct {
		edit func(d *Draft)
		want string
	}{
		{func(d *Draft) { d.Profiles[0].Was = "zzz" }, `profiles: the file has no entry "zzz"`},
		{func(d *Draft) { d.Profiles = append(d.Profiles, d.Profiles[0]) }, `profiles: entry "a" appears twice`},
		{func(d *Draft) { d.Profiles[0].Settings = "a: [" }, "profiles.a.settings: yaml:"},
		{func(d *Draft) { d.Profiles[0].Settings = "a: 1\n---\nb: 2\n" }, "profiles.a.settings: line 2: must be one YAML document"},
		{func(d *Draft) { d.Profiles[0].Settings = "a: &a [*a]\n" }, "profiles.a.settings: recursive alias *a"},
		{func(d *Draft) { d.Profiles[0].Settings = aliasBomb(12) }, "profiles.a.settings: the aliases expand to more than"},
	}
	for _, tt := range tests {
		c := d
		c.Profiles = slices.Clone(d.Profiles)
		tt.edit(&c)
		_, err := Apply([]byte(data), c)
		var es Errors
		if !errors.As(err, &es) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Apply = %v, want Errors with %q", err, tt.want)
		}
	}
}

// TestApplyRoundTrip makes many changes to the example config and checks
// that each result reads back as the draft that made it.
func TestApplyRoundTrip(t *testing.T) {
	data := string(readExample(t))
	edits := []func(d *Draft){
		func(d *Draft) { d.Providers[1].URL = "https://ollama.example" },
		func(d *Draft) { d.Models[0].Settings = "maxEffortLevel: medium\n" },
		func(d *Draft) { d.Models = slices.Delete(d.Models, 4, 5) },
		func(d *Draft) { d.Profiles[3].Subscription = "" },
		func(d *Draft) { d.Profiles[0].Env = []Pair{{"CLAUDE_CODE_EFFORT_LEVEL", "max"}} },
		func(d *Draft) { d.Defaults = Defaults{} },
		func(d *Draft) {
			d.Models[1].Name = "glm-cliproxy"
			d.Profiles[1].Model = "glm-cliproxy"
			d.Profiles[4].Aliases["haiku"] = "glm-cliproxy"
		},
		func(d *Draft) {
			d.Profiles = append(d.Profiles, Profile{Name: "kimi", Model: "kimi", Args: []string{"--verbose"}})
		},
	}
	for _, edit := range edits {
		change(t, data, false, edit)
	}
}

// TestApplyUnicodeAndCRLF: comments and values in any language survive a
// save, and a file with CRLF line endings comes out valid, with LF.
func TestApplyUnicodeAndCRLF(t *testing.T) {
	data := "# конфиг\r\nprofiles:\r\n  x: { model: opus } # основной 🚀\r\n  y: { model: sonnet }\r\n"
	out := change(t, data, true, func(d *Draft) {
		d.Profiles[1].Effort = "high"
		d.Profiles[1].Env = []Pair{{"GREETING", "привет"}}
	})
	for _, want := range []string{"# конфиг\n", "x: {model: opus} # основной 🚀\n", "GREETING: привет"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\r") {
		t.Errorf("CR left in:\n%q", out)
	}
}

// TestApplySpecialStrings: names and values that YAML would read as
// something else stay strings.
func TestApplySpecialStrings(t *testing.T) {
	out := change(t, "profiles:\n  x: { model: opus }\n", false, func(d *Draft) {
		d.Profiles = append(d.Profiles, Profile{Name: "true", Model: "opus", Env: []Pair{{"N", "0123"}, {"B", "yes"}, {"E", ""}}})
		d.Models = append(d.Models, Model{Name: "1.0", Provider: "p", ID: "1.0", Context: "100000", Label: "null"})
	})
	for _, want := range []string{`"1.0":`, `id: "1.0"`, `label: "null"`, `"true":`, `N: "0123"`, `E: ""`} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}

// TestApplyJSONSettings: JSON pasted from the Claude Code docs or an
// editor, indented with tabs, is a valid settings text.
func TestApplyJSONSettings(t *testing.T) {
	out := change(t, "profiles:\n  x: { model: opus }\n", true, func(d *Draft) {
		d.Profiles[0].Settings = "{\n\t\"autoCompactEnabled\": true,\n\t\"permissions\": {\"allow\": [\"Bash\"]}\n}\n"
	})
	if !strings.Contains(out, `settings: {"autoCompactEnabled": true, "permissions": {"allow": ["Bash"]}}`) {
		t.Errorf("got:\n%s", out)
	}
}
