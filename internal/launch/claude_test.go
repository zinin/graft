package launch

import (
	"errors"
	"os/exec"
	"path/filepath"
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
	// exec.LookPath appends the extensions of PATHEXT to claude.exe too.
	if _, err := findClaude(true, in(map[string]string{"claude.exe": `C:\npm\claude.exe.cmd`})); err == nil || !strings.Contains(err.Error(), `C:\npm\claude.exe.cmd`) {
		t.Errorf("Windows with claude.exe.cmd for claude.exe: %v", err)
	}
	if _, err := findClaude(true, in(nil)); !errors.Is(err, ErrNoClaude) {
		t.Errorf("Windows without claude: %v", err)
	}
}

// TestFindClaudeInTheCurrentFolder: exec.LookPath refuses a claude in the
// current folder with exec.ErrDot, even when PATH holds another; graft
// passes that error on rather than say that there is no claude or name a
// batch file of PATH.
func TestFindClaudeInTheCurrentFolder(t *testing.T) {
	dot := func(name string) (string, error) {
		if name == "claude.cmd" {
			return `C:\npm\claude.cmd`, nil
		}
		return filepath.Join(".", name), &exec.Error{Name: name, Err: exec.ErrDot}
	}
	if _, err := findClaude(false, dot); !errors.Is(err, exec.ErrDot) {
		t.Errorf("Unix: %v", err)
	}
	if _, err := findClaude(true, dot); !errors.Is(err, exec.ErrDot) {
		t.Errorf("Windows: %v", err)
	}
}
