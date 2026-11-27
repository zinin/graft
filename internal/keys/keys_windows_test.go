package keys

import (
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

// TestReadOnWindows: a key command runs in Windows PowerShell and means
// there what it means when typed into PowerShell, quotes, $, & and ; and
// Cyrillic included; and graft does not judge a key file by mode bits,
// which Windows does not have.
func TestReadOnWindows(t *testing.T) {
	r := Reader{}
	if key, warning, err := r.Read(config.KeySource{File: writeKey(t, "sk-file\n", 0o644)}); key != "sk-file" || warning != "" || err != nil {
		t.Errorf("key file: %q, warning %q, %v; want sk-file and no warning", key, warning, err)
	}
	t.Setenv("GRAFT_TEST_PART", "p1")
	for command, want := range map[string]string{
		`"sk-""q"""`:                    `sk-"q"`,
		`'sk-a&b;c'`:                    `sk-a&b;c`,
		`$s = 'кир'; "sk-" + $s.Length`: `sk-3`,
		`"sk-$env:GRAFT_TEST_PART"`:     `sk-p1`,
		`Write-Output 'sk-' | ForEach-Object { $_ + 'piped' }`: `sk-piped`,
	} {
		if key, _, err := r.Read(config.KeySource{Command: command}); key != want || err != nil {
			t.Errorf("command %s: %q, %v; want %q", command, key, err, want)
		}
	}
	if _, _, err := r.Read(config.KeySource{Command: "exit 3"}); err == nil || !strings.Contains(err.Error(), `command "exit 3" failed: exit status 3`) {
		t.Errorf("exit 3: %v", err)
	}
}

// TestCommandWithoutPSModulePath: PowerShell 7 puts its module folders into
// PSModulePath, and a key command runs without graft's PSModulePath, so that
// Windows PowerShell builds its own and loads its own modules.
func TestCommandWithoutPSModulePath(t *testing.T) {
	t.Setenv("PSModulePath", `C:\graft-test-marker`)
	command := `"sk-" + [int]($env:PSModulePath -like '*graft-test-marker*')`
	if key, _, err := (Reader{}).Read(config.KeySource{Command: command}); key != "sk-0" || err != nil {
		t.Errorf("%q, %v; want sk-0: the command saw graft's PSModulePath", key, err)
	}
}
