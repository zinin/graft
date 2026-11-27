package plan

import (
	"slices"
	"testing"
)

// TestConflictsInAnyCaseOnWindows: Claude Code on Windows takes a settings
// file's anthropic_base_url for ANTHROPIC_BASE_URL (seen on a Windows VM:
// the request went to that address with the provider's key), and --settings
// cancels ANTHROPIC_API_KEY and the other pinned names only as written there,
// so another spelling of them is a conflict too.
func TestConflictsInAnyCaseOnWindows(t *testing.T) {
	files := []SettingsFile{{Path: "~/.claude/settings.json", Data: map[string]any{"env": map[string]any{
		"anthropic_base_url": "http://x",
		"Anthropic_Api_Key":  "sk-ant",
		"ANTHROPIC_API_KEY":  "",
		"ANTHROPIC_MODEL":    "m",
		"OTHER":              "x",
	}}}}
	want := []string{"~/.claude/settings.json: env.anthropic_base_url", "~/.claude/settings.json: env.Anthropic_Api_Key"}
	if got := Conflicts(files); !slices.Equal(got, want) {
		t.Errorf("Conflicts = %q, want %q", got, want)
	}
}
