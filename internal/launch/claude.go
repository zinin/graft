package launch

import (
	"errors"
	"fmt"
	"runtime"
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
// that the user knows what to install.
func findClaude(windows bool, lookPath func(string) (string, error)) (string, error) {
	if !windows {
		if path, err := lookPath("claude"); err == nil {
			return path, nil
		}
		return "", ErrNoClaude
	}
	if path, err := lookPath("claude.exe"); err == nil {
		return path, nil
	}
	for _, name := range []string{"claude.cmd", "claude.bat"} {
		if path, err := lookPath(name); err == nil {
			return "", fmt.Errorf("claude in PATH is %s, a batch file, and cmd.exe would garble graft's arguments: "+
				"install Claude Code with its native installer (irm https://claude.ai/install.ps1 | iex)", path)
		}
	}
	return "", ErrNoClaude
}
