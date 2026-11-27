package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindClaudeInPathOnWindows: with the real exec.LookPath, a claude.cmd
// of npm in a folder before the one of claude.exe does not win, and alone it
// is named.
func TestFindClaudeInPathOnWindows(t *testing.T) {
	npm, native := t.TempDir(), t.TempDir()
	for _, f := range []string{filepath.Join(npm, "claude.cmd"), filepath.Join(native, "claude.exe")} {
		if err := os.WriteFile(f, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", npm+string(os.PathListSeparator)+native)
	if p, err := FindClaude(exec.LookPath); err != nil || !strings.EqualFold(p, filepath.Join(native, "claude.exe")) {
		t.Errorf("FindClaude = %q, %v; want the claude.exe", p, err)
	}
	t.Setenv("PATH", npm)
	if _, err := FindClaude(exec.LookPath); err == nil || !strings.Contains(err.Error(), "a batch file") {
		t.Errorf("FindClaude with only claude.cmd: %v", err)
	}
}
