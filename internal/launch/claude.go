package launch

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrNoClaude is the error of FindClaude when there is no claude in PATH.
var ErrNoClaude = errors.New("claude not found in PATH")

// FindClaude returns the path of the claude that graft starts, found through
// lookPath, which is exec.LookPath outside tests.
func FindClaude(lookPath func(string) (string, error)) (string, error) {
	return findClaude(runtime.GOOS == "windows", lookPath)
}

// findClaude finds claude in PATH. On Windows it takes claude.exe only:
// Windows runs a batch file, such as the claude.cmd that npm puts in PATH,
// through cmd.exe, which garbles the JSON of --settings and --agents and
// takes & or % in them for its own. A batch file found alone is named, so
// that the user knows what to install, and so is one found for claude.exe:
// exec.LookPath appends the extensions of PATHEXT to a name that has one, so
// a claude.exe.cmd in an earlier folder of PATH comes first. A claude in the
// current folder, which exec.LookPath refuses with exec.ErrDot even when PATH
// holds another, keeps that error, which says why.
func findClaude(windows bool, lookPath func(string) (string, error)) (string, error) {
	if !windows {
		path, err := lookPath("claude")
		if err == nil {
			return path, nil
		}
		if errors.Is(err, exec.ErrDot) {
			return "", err
		}
		return "", ErrNoClaude
	}
	path, err := lookPath("claude.exe")
	if err == nil {
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return "", batchFile(path)
		}
		return path, nil
	}
	if errors.Is(err, exec.ErrDot) {
		return "", err
	}
	for _, name := range []string{"claude.cmd", "claude.bat"} {
		if path, err := lookPath(name); err == nil {
			return "", batchFile(path)
		}
	}
	return "", ErrNoClaude
}

// batchFile is the error of findClaude for the batch file at path.
func batchFile(path string) error {
	return fmt.Errorf("claude in PATH is %s, a batch file, and cmd.exe would garble graft's arguments: "+
		"install Claude Code with its native installer (irm https://claude.ai/install.ps1 | iex)", path)
}
