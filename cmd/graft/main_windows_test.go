package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestConfigPathOnWindows: --config takes ~\ as ~/, and without a home
// directory the error names USERPROFILE, which Windows keeps it in.
func TestConfigPathOnWindows(t *testing.T) {
	if got, err := configPath(`~\g.yaml`, system{home: `C:\Users\u`}); err != nil || got != `C:\Users\u\g.yaml` {
		t.Errorf(`configPath(~\g.yaml) = %q, %v`, got, err)
	}
	_, err := configPath("", system{})
	if err == nil || !strings.Contains(err.Error(), "cannot find the home directory: set USERPROFILE or pass --config") {
		t.Errorf("configPath without a home: %v", err)
	}
}

// TestStartRefusesABatchClaude: a claude.cmd of npm alone in PATH would run
// through cmd.exe, which garbles the JSON of --settings and --agents, so
// graft starts nothing and says why; check says so too.
func TestStartRefusesABatchClaude(t *testing.T) {
	f := newFake(t)
	f.writeConfig(t)
	f.sys.lookPath = func(name string) (string, error) {
		if name == "claude.cmd" {
			return `C:\npm\claude.cmd`, nil
		}
		return "", exec.ErrNotFound
	}
	if code := run([]string{"glm-proxy"}, f.sys); code != 1 || !strings.Contains(f.stderr.String(), `C:\npm\claude.cmd, a batch file`) {
		t.Errorf("start: code %d, stderr %q", code, f.stderr.String())
	}
	f.stdout.Reset()
	run([]string{"check"}, f.sys)
	if !strings.Contains(f.stdout.String(), `claude: error: claude in PATH is C:\npm\claude.cmd, a batch file`) {
		t.Errorf("check:\n%s", f.stdout.String())
	}
}
