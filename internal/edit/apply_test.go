package edit

import (
	"encoding/json"
	"errors"
	"reflect"
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

// envOf returns the variables of an env as the page sends them, in JSON.
func envOf(t *testing.T, js string) []Pair {
	t.Helper()
	var env []Pair
	if err := json.Unmarshal([]byte(js), &env); err != nil {
		t.Fatal(err)
	}
	return env
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

// TestApplyUnquotesANumberOrABooleanChangedInPlace: a value changed in
// place kept the quotes it had, so context: "400000" changed to 500000 was
// written as the string "500000", and subscription: "false" changed to true
// as the string "true". config.Parse kept rejecting both, and nothing typed
// in the form could clear it. A string keeps its quotes.
func TestApplyUnquotesANumberOrABooleanChangedInPlace(t *testing.T) {
	for _, q := range []string{`"`, `'`} {
		data := `providers:
  p: {url: "https://p.example", key: {env: P_KEY}}
models:
  m:
    provider: p
    context: ` + q + `400000` + q + ` # the window
    label: ` + q + `GPT Sol` + q + `
profiles:
  x: {model: m, subscription: ` + q + `false` + q + `}
`
		out := change(t, data, true, func(d *Draft) {
			d.Models[0].Context = "500000"
			d.Models[0].Label = "GPT Terra"
			d.Profiles[0].Subscription = "true"
		})
		for _, want := range []string{
			"    context: 500000 # the window\n",
			"    label: " + q + "GPT Terra" + q + "\n",
			"x: {model: m, subscription: true}",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("quote %s: want %q in:\n%s", q, want, out)
			}
		}
	}
}

// TestApplyFixesANumberOrABooleanInQuotes: context: "400000" read as 400000
// and subscription: "true" as true, so a form could not fix them: typing the
// same text changed nothing, and config.Parse kept rejecting the file. The
// form shows the quotes, and the value typed without them is a number or a
// boolean, changed in place, so that its comment stays.
func TestApplyFixesANumberOrABooleanInQuotes(t *testing.T) {
	data := `providers:
  p: {url: "https://p.example", key: {env: P_KEY}}
models:
  m:
    provider: p
    context: "400000" # the window
profiles:
  x:
    model: m
    subscription: "true" # the login
`
	out := change(t, data, true, func(d *Draft) {
		d.Models[0].Context = "400000"
		d.Profiles[0].Subscription = "true"
	})
	for _, want := range []string{"    context: 400000 # the window\n", "    subscription: true # the login\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	// A model whose context is an alias of the one fixed keeps the old value.
	data = `providers:
  p: {url: "https://p.example", key: {env: P_KEY}}
models:
  a: {provider: p, context: &c "400000"}
  b: {provider: p, context: *c}
profiles:
  x: {model: a}
`
	out = change(t, data, false, func(d *Draft) { d.Models[0].Context = "400000" })
	for _, want := range []string{"  a: {provider: p, context: 400000}\n", "  b: {provider: p, context: \"400000\"}\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("an aliased context fixed: want %q in:\n%s", want, out)
		}
	}
}

// TestApplyReadsContextAndSubscriptionAsYAML: their text is YAML. A plain
// value takes its tag as in TestApplyContextWithALeadingZero, so spaces
// around a number leave a number, and a value in quotes or with a tag keeps
// them. A text that is not one YAML value, such as "400000 with no closing
// quote, a list or a mapping, is a string, which config.Parse reports: a list
// or a mapping in the field would keep the forms closed. Each value reads back
// as the page sent it, from a model written as a block and from one written in
// { }, where YAML puts quotes around more strings.
func TestApplyReadsContextAndSubscriptionAsYAML(t *testing.T) {
	const head = "providers:\n  p: {url: \"https://p.example\", key: {env: P_KEY}}\nmodels:\n"
	entries := []struct{ model, profile string }{
		{"  m:\n    provider: p\n    context: 100000\n", "profiles:\n  x:\n    model: m\n    subscription: false\n"},
		{"  m: {provider: p, context: 100000}\n", "profiles:\n  x: {model: m, subscription: false}\n"},
	}
	tests := []struct {
		field, text, want, problem string
	}{
		{"context", " 400000 ", `400000`, ""},
		{"context", "!!int 400000", `!!int 400000`, ""},
		{"context", "400000 # the window", `400000`, ""},
		{"context", `"400000"`, `"400000"`, "must be an integer"},
		{"context", `'400000'`, `'400000'`, "must be an integer"},
		{"context", "0400000", `"0400000"`, "must be an integer"},
		{"context", `"400000`, `'"400000'`, "must be an integer"},
		{"context", "[1, 2]", `'[1, 2]'`, "must be an integer"},
		{"context", "a: b", `'a: b'`, "must be an integer"},
		{"context", "400000\n---\n500000", `"400000\n---\n500000"`, "must be an integer"},
		{"context", "|\n  400000\n", `"400000\n"`, "must be an integer"},
		{"context", "!!null ~", `'!!null ~'`, "must be an integer"},
		{"subscription", " true ", `true`, ""},
		{"subscription", `"true"`, `"true"`, "must be true or false"},
		{"subscription", "True", `"True"`, "must be true or false"},
		{"subscription", "yes", `yes`, "must be true or false"},
		{"subscription", "[true]", `'[true]'`, "must be true or false"},
	}
	for _, e := range entries {
		for _, tt := range tests {
			out := change(t, head+e.model+e.profile, tt.problem == "", func(d *Draft) {
				if tt.field == "context" {
					d.Models[0].Context = tt.text
				} else {
					d.Profiles[0].Subscription = tt.text
				}
			})
			back, err := Read([]byte(out))
			if err != nil {
				continue // change has reported it
			}
			got := back.Models[0].Context
			if tt.field == "subscription" {
				got = back.Profiles[0].Subscription
			}
			if got != tt.want {
				t.Errorf("%s %q reads back as %q, want %q, from:\n%s", tt.field, tt.text, got, tt.want, out)
			}
			_, err = config.Parse([]byte(out), "/home/u/.config/graft/config.yaml")
			if tt.problem != "" && (err == nil || !strings.Contains(err.Error(), tt.problem)) {
				t.Errorf("%s %q: config.Parse = %v, want %q", tt.field, tt.text, err, tt.problem)
			}
		}
		// These read back with quotes from { } and without them from a block,
		// or, for <<, which YAML reads as a merge key, with the tag of one.
		for _, text := range []string{"1,000,000", "a:b", "!!int", "<<"} {
			change(t, head+e.model+e.profile, false, func(d *Draft) { d.Models[0].Context = text })
			change(t, head+e.model+e.profile, false, func(d *Draft) { d.Profiles[0].Subscription = text })
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

// TestApplyRemovesTheCommentsBelowTheLastEntry: deleting the last entry of a
// section also deletes the comment lines right below it, which yaml.v3 holds
// as a part of that entry, as README says.
func TestApplyRemovesTheCommentsBelowTheLastEntry(t *testing.T) {
	data := "profiles:\n  a:\n    model: opus\n  b:\n    model: sonnet\n  # c: {model: haiku}\n"
	out := change(t, data, true, func(d *Draft) { d.Profiles = d.Profiles[:1] })
	if want := "profiles:\n  a:\n    model: opus\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
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

// TestApplyKeepsTheListItemsThatStay: a changed list was built anew, so
// adding an argument dropped the comments of the arguments already there,
// and their anchors. The items that stay keep their nodes.
func TestApplyKeepsTheListItemsThatStay(t *testing.T) {
	data := `providers:
  p: {url: "https://p.example", key: {env: P_KEY}}
models:
  a: {provider: p, context: 100000}
  b: {provider: p, context: 100000}
  c: {provider: p, context: 100000}
defaults:
  args:
    - --one # why one
    - --two
profiles:
  x:
    model: a
    picker:
      - a # fast
      - b # cheap
`
	tests := []struct {
		name string
		edit func(d *Draft)
		want string
	}{
		{"an item added", func(d *Draft) { d.Defaults.Args = append(d.Defaults.Args, "--three") },
			"  args:\n    - --one # why one\n    - --two\n    - --three\n"},
		{"another item removed", func(d *Draft) { d.Defaults.Args = []string{"--one"} },
			"  args:\n    - --one # why one\n"},
		{"two items swapped", func(d *Draft) { d.Defaults.Args = []string{"--two", "--one"} },
			"  args:\n    - --two\n    - --one # why one\n"},
		{"a model added to picker", func(d *Draft) { d.Profiles[0].Picker = append(d.Profiles[0].Picker, "c") },
			"    picker:\n      - a # fast\n      - b # cheap\n      - c\n"},
	}
	for _, tt := range tests {
		if out := change(t, data, true, tt.edit); !strings.Contains(out, tt.want) {
			t.Errorf("%s: want %q in:\n%s", tt.name, tt.want, out)
		}
	}
	// Each of two equal items keeps a node of its own.
	data = "profiles:\n  x:\n    model: opus\n    args:\n      - --v # first\n      - --v # second\n"
	out := change(t, data, true, func(d *Draft) { d.Profiles[0].Args = append(d.Profiles[0].Args, "--w") })
	if want := "    args:\n      - --v # first\n      - --v # second\n      - --w\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	// An anchored item keeps its anchor, and an alias stays an alias.
	data = "defaults:\n  args: [&v --verbose]\nprofiles:\n  x:\n    model: opus\n    args: [*v, --two]\n"
	out = change(t, data, true, func(d *Draft) {
		d.Defaults.Args = append(d.Defaults.Args, "--one")
		d.Profiles[0].Args = append(d.Profiles[0].Args, "--three")
	})
	for _, want := range []string{"  args: [&v --verbose, --one]\n", "    args: [*v, --two, --three]\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
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
		d.Profiles[0].Env = []Pair{{Name: "KEEP", Value: "1"}, {Name: "CHANGE", Value: ""}, {Name: "NEW", Value: "0"}}
	})
	if want := "    env:\n      KEEP: \"1\" # stays\n      CHANGE: \"\"\n      NEW: \"0\"\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Env = nil })
	if strings.Contains(out, "env") {
		t.Errorf("env was not removed:\n%s", out)
	}
}

// TestApplyChangesAnEnvValueInPlace: a changed variable got a new value
// node, which dropped the comment on the value.
func TestApplyChangesAnEnvValueInPlace(t *testing.T) {
	data := "profiles:\n  x:\n    model: opus\n    env:\n      LEVEL: high # the default is low\n"
	out := change(t, data, true, func(d *Draft) { d.Profiles[0].Env = []Pair{{Name: "LEVEL", Value: "max"}} })
	if want := "    env:\n      LEVEL: max # the default is low\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
}

// TestApplyGivesAValueToAVariableWithNone: a variable the file gives no
// value, such as FOO:, read as the empty string, so the empty string sent
// for it changed nothing, and config.Parse kept rejecting the file. Now it
// has no value, null in JSON: the empty string is a change, made in place,
// so that the comment stays, and a variable that keeps no value keeps its
// nodes. A new variable with no value, which only the API can send, is
// written with an empty value, which config.Parse reports, and so is a value
// the API takes away.
func TestApplyGivesAValueToAVariableWithNone(t *testing.T) {
	data := `profiles:
  x:
    model: opus
    env:
      KEEP: # none yet
      FIX: ~ # the empty string
      DROP: a # its value
`
	out := change(t, data, false, func(d *Draft) {
		d.Profiles[0].Env = envOf(t, `[["KEEP", null], ["FIX", ""], ["DROP", null], ["NEW", null]]`)
	})
	if want := "    env:\n      KEEP: # none yet\n      FIX: \"\" # the empty string\n      DROP: # its value\n      NEW:\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	_, err := config.Parse([]byte(out), "/home/u/.config/graft/config.yaml")
	for _, name := range []string{"KEEP", "DROP", "NEW"} {
		if want := "profiles.x.env." + name + `: must be a string: write "" for an empty value`; err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("config.Parse = %v, want %q", err, want)
		}
	}
	if err != nil && strings.Contains(err.Error(), "FIX") {
		t.Errorf("config.Parse = %v, want nothing about FIX", err)
	}
	// In { }, YAML writes an empty value as null.
	out = change(t, "profiles:\n  x: {model: opus, env: {KEEP: , FIX: ~}}\n", false, func(d *Draft) {
		d.Profiles[0].Env = envOf(t, `[["KEEP", null], ["FIX", ""], ["NEW", null]]`)
	})
	if want := "  x: {model: opus, env: {KEEP: null, FIX: \"\", NEW: null}}\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
}

// TestApplyKeepsTheAnchorsAndAliasesOfEnv: a change of env wrote the value
// of an alias that did not change, and took the anchor from a value that did
// not change, making its aliases elsewhere copies. A change still never
// reaches another field through an alias.
func TestApplyKeepsTheAnchorsAndAliasesOfEnv(t *testing.T) {
	data := `providers:
  p: {url: &u "https://p.example", key: {env: P_KEY}}
models:
  m: {provider: p, context: 100000}
profiles:
  x:
    model: m
    env:
      FOO: &f x
      URL: *u
  y:
    model: m
    env: {BAR: *f}
`
	out := change(t, data, true, func(d *Draft) { d.Profiles[0].Env = append(d.Profiles[0].Env, Pair{Name: "NEW", Value: "1"}) })
	for _, want := range []string{"    env:\n      FOO: &f x\n      URL: *u\n      NEW: \"1\"\n", "    env: {BAR: *f}\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("a variable added: want %q in:\n%s", want, out)
		}
	}
	// The anchored value changes: its alias gets the old value.
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Env[0].Value = "z" })
	for _, want := range []string{"    env:\n      FOO: z\n      URL: *u\n", "    env: {BAR: x}\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("the anchored value changed: want %q in:\n%s", want, out)
		}
	}
	// A profile whose env is an alias of the env that changes keeps the old one.
	data = `profiles:
  x:
    model: opus
    env: &e {A: "1"}
  y:
    model: opus
    env: *e
`
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Env = []Pair{{Name: "A", Value: "2"}} })
	if want := "  x:\n    model: opus\n    env: {A: \"2\"}\n  y:\n    model: opus\n    env: {A: \"1\"}\n"; !strings.Contains(out, want) {
		t.Errorf("an aliased env changed: want %q in:\n%s", want, out)
	}
}

// TestApplyCopiesAnEnvAliasPutBeforeItsAnchor: env keeps the nodes of the
// variables that stay, in the new order, so {A: &a "1", B: *a} with A
// deleted and added again after B became {B: *a, A: &a "1"}, which YAML does
// not read: the anchor comes after its alias. Such an alias becomes a copy
// of the value, with its own comments; the anchor stays for the aliases
// after it.
func TestApplyCopiesAnEnvAliasPutBeforeItsAnchor(t *testing.T) {
	data := `profiles:
  x:
    model: opus
    env: {A: &a "1", B: *a}
  y:
    model: opus
    env: {C: *a}
`
	reorder := func(d *Draft) { d.Profiles[0].Env = []Pair{{Name: "B", Value: "1"}, {Name: "A", Value: "1"}} }
	out := change(t, data, true, reorder)
	for _, want := range []string{"    env: {B: \"1\", A: &a \"1\"}\n", "    env: {C: *a}\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	data = "profiles:\n  x:\n    model: opus\n    env:\n      A: &a \"1\" # the level\n      B: *a # as A\n"
	out = change(t, data, true, reorder)
	if want := "    env:\n      B: \"1\" # as A\n      A: &a \"1\" # the level\n"; !strings.Contains(out, want) {
		t.Errorf("the comments of the alias: want %q in:\n%s", want, out)
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

// TestApplyChangesAKeyInPlace: a key source was built anew even when only
// its value changed, which dropped the comments on it. Another kind of
// source still takes the place of the old one, in its style.
func TestApplyChangesAKeyInPlace(t *testing.T) {
	data := `providers:
  p:
    url: https://p.example
    key:
      # the file graft reads
      file: keys/p # chmod 600
models:
  m: {provider: p, context: 100000}
profiles:
  x: {model: m}
`
	out := change(t, data, true, func(d *Draft) { d.Providers[0].Key.Value = "keys/other" })
	if want := "    key:\n      # the file graft reads\n      file: keys/other # chmod 600\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	out = change(t, data, true, func(d *Draft) { d.Providers[0].Key = Key{Kind: "env", Value: "P_KEY"} })
	if want := "    key:\n      env: P_KEY\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	// A provider whose key is an alias of the one that changes keeps the old source.
	data = `providers:
  a: {url: "https://a.example", key: &k {file: keys/a}}
  b: {url: "https://b.example", key: *k}
models:
  m: {provider: a, context: 100000}
profiles:
  x: {model: m}
`
	out = change(t, data, true, func(d *Draft) { d.Providers[0].Key.Value = "keys/new" })
	for _, want := range []string{"  a: {url: \"https://a.example\", key: {file: keys/new}}\n", "  b: {url: \"https://b.example\", key: {file: keys/a}}\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("an aliased key changed: want %q in:\n%s", want, out)
		}
	}
}

// TestApplyKeepsTheUnknownFieldsOfAKey: a key with a field the draft does
// not know, such as {env: TOKEN, future_option: value}, was built anew when
// its source changed, which dropped the field. The field stays in its place,
// and config.Parse keeps reporting it, so a save waits until it is removed
// by hand. Of the sources graft knows, only the chosen one stays.
func TestApplyKeepsTheUnknownFieldsOfAKey(t *testing.T) {
	const rest = "models:\n  m: {provider: p, context: 100000}\nprofiles:\n  x: {model: m}\n"
	data := "providers:\n  p: {url: \"https://p.example\", key: {env: TOKEN, future_option: value}}\n" + rest
	out := change(t, data, false, func(d *Draft) { d.Providers[0].Key.Value = "OTHER" })
	if want := "key: {env: OTHER, future_option: value}}\n"; !strings.Contains(out, want) {
		t.Errorf("the value changed: want %q in:\n%s", want, out)
	}
	if _, err := config.Parse([]byte(out), "/home/u/.config/graft/config.yaml"); err == nil || !strings.Contains(err.Error(), "future_option") {
		t.Errorf("config.Parse = %v, want an error about future_option", err)
	}
	out = change(t, data, false, func(d *Draft) { d.Providers[0].Key = Key{Kind: "file", Value: "keys/p"} })
	if want := "key: {file: keys/p, future_option: value}}\n"; !strings.Contains(out, want) {
		t.Errorf("another source: want %q in:\n%s", want, out)
	}
	data = "providers:\n  p: {url: \"https://p.example\", key: {env: A, file: B, extra: 1}}\n" + rest
	out = change(t, data, false, func(d *Draft) { d.Providers[0].Key = Key{Kind: "command", Value: "pass show p"} })
	if want := "key: {command: pass show p, extra: 1}}\n"; !strings.Contains(out, want) {
		t.Errorf("two sources: want %q in:\n%s", want, out)
	}
	data = `providers:
  p:
    url: https://p.example
    key:
      env: TOKEN
      # for graft 2
      future_option: value # not read yet
` + rest
	out = change(t, data, false, func(d *Draft) { d.Providers[0].Key.Value = "OTHER" })
	if want := "    key:\n      env: OTHER\n      # for graft 2\n      future_option: value # not read yet\n"; !strings.Contains(out, want) {
		t.Errorf("the comments of the field: want %q in:\n%s", want, out)
	}
	// A provider whose key is an alias gets a mapping of its own, with the
	// field; the anchored mapping stays for the other aliases.
	data = `providers:
  a: {url: "https://a.example", key: &k {env: T, extra: 1}}
  b: {url: "https://b.example", key: *k}
models:
  m: {provider: a, context: 100000}
profiles:
  x: {model: m}
`
	out = change(t, data, false, func(d *Draft) { d.Providers[1].Key.Value = "U" })
	for _, want := range []string{"  a: {url: \"https://a.example\", key: &k {env: T, extra: 1}}\n", "  b: {url: \"https://b.example\", key: {env: U, extra: 1}}\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("an alias of the key: want %q in:\n%s", want, out)
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

// TestApplyFillsAnAliasOfANull: an entry, a section or a field that is an
// alias of a null becomes a mapping of its own when it gets a field, as the
// null itself does, and the null stays.
func TestApplyFillsAnAliasOfANull(t *testing.T) {
	tests := []struct {
		name, data string
		valid      bool
		edit       func(d *Draft)
		want       string
	}{
		{"entry", "profiles:\n  a: &x\n  b: *x\n", false,
			func(d *Draft) { d.Profiles[1].Model = "opus" },
			"profiles:\n  a: &x\n  b:\n    model: opus\n"},
		{"section", "models: &n\ndefaults: *n\nprofiles:\n  x: {model: opus}\n", true,
			func(d *Draft) { d.Defaults.Effort = "max" },
			"models: &n\n\ndefaults:\n  effort: max\n\nprofiles:\n  x: {model: opus}\n"},
		{"field", "profiles:\n  a:\n    model: opus\n    aliases: &n\n  b:\n    model: opus\n    aliases: *n\n", true,
			func(d *Draft) { d.Profiles[1].Aliases["haiku"] = "claude-haiku-5-5" },
			"profiles:\n  a:\n    model: opus\n    aliases: &n\n  b:\n    model: opus\n    aliases:\n      haiku: claude-haiku-5-5\n"},
	}
	for _, tt := range tests {
		if out := change(t, tt.data, tt.valid, tt.edit); out != tt.want {
			t.Errorf("%s: got:\n%s\nwant:\n%s", tt.name, out, tt.want)
		}
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
		{func(d *Draft) { d.Profiles[0].Settings = "a: [" }, "profiles.a.settings: line 1: did not find expected node content"},
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
		func(d *Draft) { d.Profiles[0].Env = []Pair{{Name: "CLAUDE_CODE_EFFORT_LEVEL", Value: "max"}} },
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
		d.Profiles[1].Env = []Pair{{Name: "GREETING", Value: "привет"}}
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
		d.Profiles = append(d.Profiles, Profile{Name: "true", Model: "opus",
			Env: []Pair{{Name: "N", Value: "0123"}, {Name: "B", Value: "yes"}, {Name: "E", Value: ""}}})
		d.Models = append(d.Models, Model{Name: "1.0", Provider: "p", ID: "1.0", Context: "100000", Label: "null"})
	})
	for _, want := range []string{`"1.0":`, `id: "1.0"`, `label: "null"`, `"true":`, `N: "0123"`, `E: ""`} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}

// TestApplyWritesAnEmptyValueInAFlowCollectionAsNull: yaml.v3 writes the
// empty null of {env: } as an empty string in quotes. A save of another
// entry then made a valid env one that config.Parse rejects, and turned the
// settings {a: } into {a: ""}, another value. The value is written as null
// instead.
func TestApplyWritesAnEmptyValueInAFlowCollectionAsNull(t *testing.T) {
	const head = `providers:
  p: {url: "https://p.example", key: {env: P_KEY}}
models:
  m: {provider: p, context: 100000}
profiles:
`
	tests := []struct {
		name, profile, want string
		settings            map[string]any // what config.Parse must read as the settings of x
	}{
		{"env", "  x: {model: m, env: }\n", "  x: {model: m, env: null}\n", nil},
		{"settings", "  x: {model: m, settings: {a: }}\n", "  x: {model: m, settings: {a: null}}\n", map[string]any{"a": nil}},
		{"nested, anchored and aliased", "  x: {model: m, settings: {a: &z , b: *z, c: [1, {d: }]}}\n",
			"  x: {model: m, settings: {a: &z null, b: *z, c: [1, {d: null}]}}\n",
			map[string]any{"a": nil, "b": nil, "c": []any{1, map[string]any{"d": nil}}}},
	}
	for _, tt := range tests {
		data := head + tt.profile + "  y: {model: m}\n"
		// A change of y: the file's x is not touched, yet rewritten.
		out := change(t, data, true, func(d *Draft) { d.Profiles[1].Effort = "high" })
		if !strings.Contains(out, tt.want) {
			t.Errorf("%s: want %q in:\n%s", tt.name, tt.want, out)
		}
		cfg, err := config.Parse([]byte(out), "/home/u/.config/graft/config.yaml")
		if err != nil {
			continue // change has reported it
		}
		if got := cfg.Profiles["x"].Settings; tt.settings != nil && !reflect.DeepEqual(got, tt.settings) {
			t.Errorf("%s: the settings of x are %#v, want %#v", tt.name, got, tt.settings)
		}
	}
}

// TestApplyWritesATypedEmptyValueAsNull: the settings text a:\nb: 1 holds an
// empty null. Put into a profile written in flow style, like x, it would
// become an empty string, and the save would not come out as the page
// showed it.
func TestApplyWritesATypedEmptyValueAsNull(t *testing.T) {
	out := change(t, "profiles:\n  x: {model: opus}\n", true, func(d *Draft) { d.Profiles[0].Settings = "a:\nb: 1\n" })
	if want := "  x: {model: opus, settings: {a: null, b: 1}}\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
}

// TestApplyKeepsAnEmptyValueInBlockStyle: block style writes nothing for an
// empty null, which reads back as null, so it keeps its look.
func TestApplyKeepsAnEmptyValueInBlockStyle(t *testing.T) {
	const data = `providers:
  p:
    url: https://p.example
    key: {env: P_KEY}

models:
  m:
    provider: p
    context: 100000

profiles:
  x:
    model: m
    env:
    settings:
      a:
      b: 1
  y:
    model: m
`
	out := change(t, data, true, func(d *Draft) { d.Profiles[1].Effort = "high" })
	if want := data + "    effort: high\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
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
