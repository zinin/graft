package launch

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestFindClaude: on Windows graft runs claude.exe only, even when a
// claude.cmd of npm comes first in PATH, and names a batch file it finds
// alone, which cmd.exe would run with graft's arguments garbled.
func TestFindClaude(t *testing.T) {
	in := func(found map[string]string) func(string) (string, error) {
		return func(name string) (string, error) {
			if p, ok := found[name]; ok {
				return p, nil
			}
			return "", exec.ErrNotFound
		}
	}
	if p, err := findClaude(false, in(map[string]string{"claude": "/bin/claude"})); p != "/bin/claude" || err != nil {
		t.Errorf("Unix: %q, %v", p, err)
	}
	if _, err := findClaude(false, in(nil)); !errors.Is(err, ErrNoClaude) {
		t.Errorf("Unix without claude: %v", err)
	}
	both := map[string]string{"claude.exe": `C:\u\.local\bin\claude.exe`, "claude.cmd": `C:\npm\claude.cmd`}
	if p, err := findClaude(true, in(both)); p != `C:\u\.local\bin\claude.exe` || err != nil {
		t.Errorf("Windows with claude.exe and claude.cmd: %q, %v", p, err)
	}
	for _, name := range []string{"claude.cmd", "claude.bat"} {
		_, err := findClaude(true, in(map[string]string{name: `C:\npm\` + name}))
		if err == nil || !strings.Contains(err.Error(), `C:\npm\`+name+", a batch file") || !strings.Contains(err.Error(), "native installer") {
			t.Errorf("Windows with %s alone: %v", name, err)
		}
	}
	if _, err := findClaude(true, in(nil)); !errors.Is(err, ErrNoClaude) {
		t.Errorf("Windows without claude: %v", err)
	}
}

// claudeFile is the name graft asks PATH for: claude.exe on Windows, where
// a claude.cmd must not run.
func claudeFile() string {
	if runtime.GOOS == "windows" {
		return "claude.exe"
	}
	return "claude"
}
