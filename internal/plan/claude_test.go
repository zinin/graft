package plan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadClaudeSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"env": {"ANTHROPIC_BASE_URL": "https://gateway.example", "ANTHROPIC_AUTH_TOKEN": "", "OTHER": "x"}}`)
	writeFile(t, filepath.Join(project, ".claude", "settings.local.json"), `{"apiKeyHelper": "get-key"}`)
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{not json`)

	files, warnings := LoadClaudeSettings(filepath.Join(home, ".claude"), project)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2: %+v", len(files), files)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "cannot parse") {
		t.Errorf("warnings = %q, want one about the broken file", warnings)
	}
	want := []string{
		"~/.claude/settings.json: env.ANTHROPIC_BASE_URL",
		"~/.claude/settings.json: env.ANTHROPIC_AUTH_TOKEN",
		filepath.Join(project, ".claude", "settings.local.json") + ": apiKeyHelper",
	}
	if got := Conflicts(files); !reflect.DeepEqual(got, want) {
		t.Errorf("Conflicts = %q, want %q", got, want)
	}
}

// TestEmptyValuesAreConflicts: an empty string in a settings file's env
// cancels the variable that graft put into the environment, so even an empty
// ANTHROPIC_AUTH_TOKEN would leave the provider's address without its key,
// and Claude Code would send the claude.ai login there.
func TestEmptyValuesAreConflicts(t *testing.T) {
	files := []SettingsFile{
		{Path: "a", Data: map[string]any{"env": map[string]any{"ANTHROPIC_BASE_URL": "", "ANTHROPIC_AUTH_TOKEN": ""}}},
		{Path: "b", Data: map[string]any{"apiKeyHelper": "", "env": map[string]any{"ANTHROPIC_CUSTOM_HEADERS": ""}}},
	}
	want := []string{"a: env.ANTHROPIC_BASE_URL", "a: env.ANTHROPIC_AUTH_TOKEN", "b: apiKeyHelper", "b: env.ANTHROPIC_CUSTOM_HEADERS"}
	if got := Conflicts(files); !reflect.DeepEqual(got, want) {
		t.Errorf("Conflicts = %q, want %q", got, want)
	}
}

func TestUserSettingsDir(t *testing.T) {
	parse := func(src string) *config.Config {
		cfg, err := config.Parse([]byte(src), filepath.Join(t.TempDir(), "config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	withDefaults := parse(`
defaults:
  env: { CLAUDE_CONFIG_DIR: /from-defaults }
profiles:
  plain: { model: opus }
  own: { model: opus, env: { CLAUDE_CONFIG_DIR: /from-profile } }
`)
	bare := parse("profiles:\n  plain: { model: opus }\n")
	// claude gets its home directory from these env too.
	homes := parse(`
defaults:
  env: { HOME: /home/defaults }
profiles:
  plain: { model: opus }
  own: { model: opus, env: { HOME: /home/profile } }
  empty: { model: opus, env: { HOME: "" } }
`)
	profileHome := parse("profiles:\n  own: { model: opus, env: { HOME: /home/profile } }\n")
	env := []string{"CLAUDE_CONFIG_DIR=/from-env"}
	tests := []struct {
		name    string
		cfg     *config.Config
		profile string
		environ []string
		home    string
		want    string // empty: an error
	}{
		{"profile env wins", withDefaults, "own", env, "/home/u", "/from-profile"},
		{"defaults env beats the environment", withDefaults, "plain", env, "/home/u", "/from-defaults"},
		{"unknown profile uses defaults", withDefaults, "", env, "/home/u", "/from-defaults"},
		{"environment", bare, "plain", env, "/home/u", "/from-env"},
		{"home", bare, "plain", nil, "/home/u", "/home/u/.claude"},
		{"no home needed", withDefaults, "plain", nil, "", "/from-defaults"},
		{"no home", bare, "plain", nil, "", ""},
		{"HOME in the profile's env", profileHome, "own", nil, "/home/u", "/home/profile/.claude"},
		{"HOME in the env of defaults", homes, "plain", nil, "/home/u", "/home/defaults/.claude"},
		{"profile HOME beats defaults HOME", homes, "own", nil, "/home/u", "/home/profile/.claude"},
		{"empty HOME in env", homes, "empty", nil, "/home/u", ""},
	}
	for _, tt := range tests {
		got, err := UserSettingsDir(tt.cfg, tt.profile, tt.environ, tt.home)
		if tt.want == "" {
			if err == nil || !strings.Contains(err.Error(), "cannot find the home directory") {
				t.Errorf("%s: %q, %v; want an error", tt.name, got, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: %q, %v; want %q", tt.name, got, err, tt.want)
		}
	}
}

func TestProjectInHomeIsReadOnce(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"apiKeyHelper": "get-key"}`)
	files, _ := LoadClaudeSettings(filepath.Join(home, ".claude"), home)
	if len(files) != 1 {
		t.Errorf("got %d files, want the user settings once", len(files))
	}
}

func TestMissingFilesAreSkipped(t *testing.T) {
	files, warnings := LoadClaudeSettings(filepath.Join(t.TempDir(), ".claude"), t.TempDir())
	if len(files) != 0 || len(warnings) != 0 {
		t.Errorf("files %v, warnings %v, want none", files, warnings)
	}
}

// TestSavedModel: Enter in /model saves the model for new sessions in the
// user settings. A model only graft serves would send a plain claude to
// Anthropic with a name it does not know.
func TestSavedModel(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
models:
  sol:   { provider: p, id: gpt-6.1-sol, context: 400000 }
  relay: { provider: p, id: claude-opus-5-5, context: 200000 }
profiles:
  sol: { model: sol }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		settings string // "" for no file
		want     string // the model to warn about; "" for none
	}{
		{"graft name", `{"model": "sol"}`, "sol"},
		{"ID of a direct profile", `{"model": "gpt-6.1-sol"}`, "gpt-6.1-sol"},
		{"Claude alias", `{"model": "opus"}`, ""},
		// Also the relay's ID, which Anthropic serves too.
		{"Claude ID", `{"model": "claude-opus-5-5"}`, ""},
		{"model of nobody here", `{"model": "gpt-4o"}`, ""},
		{"no model", `{"theme": "dark"}`, ""},
		{"model not a string", `{"model": 1}`, ""},
		{"broken file", `{`, ""},
		{"no file", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, ".claude")
			if tt.settings != "" {
				writeFile(t, filepath.Join(dir, "settings.json"), tt.settings)
			}
			model, file := SavedModel(cfg, dir)
			if model != tt.want {
				t.Errorf("model %q, want %q", model, tt.want)
			}
			if tt.want != "" && file != "~/.claude/settings.json" {
				t.Errorf("file %q", file)
			}
		})
	}
}

// TestPlainSettingsDir: a plain claude started from graft's shell reads the
// user settings in CLAUDE_CONFIG_DIR of that shell, else in ~/.claude; what
// a profile or the defaults set for graft's sessions does not reach it.
func TestPlainSettingsDir(t *testing.T) {
	tests := []struct {
		name, home string
		environ    []string
		want       string
	}{
		{"CLAUDE_CONFIG_DIR", "/home/u", []string{"CLAUDE_CONFIG_DIR=/srv/claude"}, "/srv/claude"},
		{"home", "/home/u", nil, "/home/u/.claude"},
		{"empty CLAUDE_CONFIG_DIR", "/home/u", []string{"CLAUDE_CONFIG_DIR="}, "/home/u/.claude"},
		{"neither", "", nil, ""},
	}
	for _, tt := range tests {
		if got := PlainSettingsDir(tt.environ, tt.home); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestSavedModelWithoutADirectory: with no directory known, nothing is
// read, not even settings.json in the current directory.
func TestSavedModelWithoutADirectory(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
models:
  sol: { provider: p, context: 400000 }
profiles:
  sol: { model: sol }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, "settings.json"), `{"model": "sol"}`)
	t.Chdir(cwd)
	if model, file := SavedModel(cfg, ""); model != "" {
		t.Errorf("read %s, got %q", file, model)
	}
}

func TestLoadSettingsFor(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  ollama: { url: https://ollama.com, key: { env: OLLAMA_API_KEY } }
models:
  glm: { provider: ollama, context: 1048576 }
profiles:
  glm: { model: glm }
  opus: { model: opus }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	home, dir := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"apiKeyHelper": "get-key"}`)
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), `{not json`)

	files, warnings, err := LoadSettingsFor(cfg, "glm", nil, home, dir)
	if err != nil || len(files) != 1 || len(warnings) != 1 || !strings.Contains(warnings[0], "cannot parse") {
		t.Errorf("glm: %d files, warnings %q, %v; want the user settings and a warning about the broken file", len(files), warnings, err)
	}
	// Only a profile with external models needs the files, and a home.
	for _, profile := range []string{"opus", "none"} {
		if files, warnings, err := LoadSettingsFor(cfg, profile, nil, "", dir); files != nil || warnings != nil || err != nil {
			t.Errorf("%s: %v, %q, %v; want nothing", profile, files, warnings, err)
		}
	}
	if _, _, err := LoadSettingsFor(cfg, "glm", nil, "", dir); err == nil || !strings.Contains(err.Error(), "cannot find the home directory") {
		t.Errorf("glm without a home: %v", err)
	}
}
