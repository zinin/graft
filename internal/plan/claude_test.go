package plan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

	files, warnings := LoadClaudeSettings(nil, home, project)
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

func TestClaudeConfigDir(t *testing.T) {
	home, custom := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"apiKeyHelper": "ignored"}`)
	writeFile(t, filepath.Join(custom, "settings.json"), `{"env": {"ANTHROPIC_CUSTOM_HEADERS": "X-A: b"}}`)

	files, _ := LoadClaudeSettings([]string{"CLAUDE_CONFIG_DIR=" + custom}, home, t.TempDir())
	got := Conflicts(files)
	if len(got) != 1 || !strings.HasSuffix(got[0], "settings.json: env.ANTHROPIC_CUSTOM_HEADERS") {
		t.Errorf("Conflicts = %q, want only the file in CLAUDE_CONFIG_DIR", got)
	}
}

func TestProjectInHomeIsReadOnce(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"apiKeyHelper": "get-key"}`)
	files, _ := LoadClaudeSettings(nil, home, home)
	if len(files) != 1 {
		t.Errorf("got %d files, want the user settings once", len(files))
	}
}

func TestMissingFilesAreSkipped(t *testing.T) {
	files, warnings := LoadClaudeSettings(nil, t.TempDir(), t.TempDir())
	if len(files) != 0 || len(warnings) != 0 {
		t.Errorf("files %v, warnings %v, want none", files, warnings)
	}
}
