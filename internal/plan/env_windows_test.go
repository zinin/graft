package plan

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

// TestUnsetInAnyCaseOnWindows: a variable graft manages leaves claude's
// environment in any case, since Windows and Claude Code read it so.
func TestUnsetInAnyCaseOnWindows(t *testing.T) {
	p, err := Build(Input{Config: loadConfig(t), Profile: "kimi", Environ: []string{"anthropic_api_key=x", "Anthropic_Model=y"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_MODEL"} {
		if !slices.Contains(p.Unset, name) {
			t.Errorf("Unset = %q, want it to hold %s", p.Unset, name)
		}
	}
}

// TestNoProxyInAnyCaseOnWindows: a No_Proxy of the profile or of the shell
// is the NO_PROXY that graft extends with the router's host; it must not
// replace graft's value.
func TestNoProxyInAnyCaseOnWindows(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  cliproxy: { url: "http://127.0.0.1:8317", key: { file: keys/cliproxy } }
models:
  sol: { provider: cliproxy, id: gpt-6.1-sol, context: 400000 }
profiles:
  p: { model: sol, env: { No_Proxy: lab.example } }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Build(Input{Config: cfg, Profile: "p", Environ: []string{"no_proxy=corp.example"}})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, e := range p.Set {
		if e.Name == "No_Proxy" {
			t.Errorf("the profile's No_Proxy reaches claude as it was: %+v", e)
		}
		if !strings.EqualFold(e.Name, "NO_PROXY") {
			continue
		}
		seen++
		for _, host := range []string{"corp.example", "lab.example", "127.0.0.1"} {
			if !strings.Contains(e.Value, host) {
				t.Errorf("%s=%s lacks %s", e.Name, e.Value, host)
			}
		}
	}
	if seen == 0 {
		t.Errorf("no NO_PROXY in %+v", p.Set)
	}
}

// TestUserSettingsDirOnWindows: Claude Code takes its home directory from
// USERPROFILE on Windows and never looks at HOME.
func TestUserSettingsDirOnWindows(t *testing.T) {
	cfg, err := config.Parse([]byte(`
profiles:
  up: { model: opus, env: { USERPROFILE: 'C:\p' } }
  lower: { model: opus, env: { userprofile: 'C:\q' } }
  home: { model: opus, env: { HOME: 'C:\h' } }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ profile, home, want string }{
		{"up", `C:\u`, `C:\p\.claude`},
		{"lower", `C:\u`, `C:\q\.claude`},
		{"home", `C:\u`, `C:\u\.claude`},
	} {
		if got, err := UserSettingsDir(cfg, tt.profile, nil, tt.home); err != nil || got != tt.want {
			t.Errorf("%s: %q, %v; want %q", tt.profile, got, err, tt.want)
		}
	}
	if _, err := UserSettingsDir(cfg, "home", nil, ""); err == nil || !strings.Contains(err.Error(), "set USERPROFILE") {
		t.Errorf("no home: %v, want an error that names USERPROFILE", err)
	}
}

// TestProfileEnvReplacesDefaultsInAnyCaseOnWindows: Foo in the profile's env
// is the FOO of defaults on Windows, and the profile's value wins.
func TestProfileEnvReplacesDefaultsInAnyCaseOnWindows(t *testing.T) {
	cfg, err := config.Parse([]byte(`
defaults:
  env: { FOO: from-defaults }
profiles:
  p: { model: opus, env: { Foo: from-profile } }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Build(Input{Config: cfg, Profile: "p"})
	if err != nil {
		t.Fatal(err)
	}
	var got []EnvVar
	for _, e := range p.Set {
		if strings.EqualFold(e.Name, "FOO") {
			got = append(got, e)
		}
	}
	if len(got) != 1 || got[0].Value != "from-profile" {
		t.Errorf("FOO in claude's environment: %+v, want the profile's value once", got)
	}
}

// TestEffortLevelInAnyCaseOnWindows: a claude_code_effort_level in the
// profile's env locks the effort on Windows as CLAUDE_CODE_EFFORT_LEVEL does,
// so --settings passes its value rather than cancel it.
func TestEffortLevelInAnyCaseOnWindows(t *testing.T) {
	cfg, err := config.Parse([]byte(`
profiles:
  p: { model: opus, effort: high, env: { claude_code_effort_level: max } }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Build(Input{Config: cfg, Profile: "p"})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := p.Settings["env"].(map[string]any)
	if got := env["CLAUDE_CODE_EFFORT_LEVEL"]; got != "max" {
		t.Errorf("--settings env CLAUDE_CODE_EFFORT_LEVEL = %#v, want the profile's max", got)
	}
}
