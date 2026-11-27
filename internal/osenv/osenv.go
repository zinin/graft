// Package osenv compares and finds environment variables as the system does:
// by exact name on Unix, and on Windows in any case, where PATH and Path are
// one variable. It also names the variable that holds the home directory of
// Claude Code.
package osenv

import (
	"runtime"
	"strings"
)

// Equal reports whether a and b name the same variable.
func Equal(a, b string) bool {
	return equal(runtime.GOOS == "windows", a, b)
}

// Lookup returns the value of the first variable in environ, a list of
// NAME=value as from os.Environ, whose name is Equal to name.
func Lookup(environ []string, name string) (string, bool) {
	return lookup(runtime.GOOS == "windows", environ, name)
}

// HomeVar returns the variable Claude Code takes its home directory from:
// USERPROFILE on Windows, where it does not look at HOME, and HOME elsewhere.
func HomeVar() string {
	return homeVar(runtime.GOOS == "windows")
}

func equal(windows bool, a, b string) bool {
	if windows {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func lookup(windows bool, environ []string, name string) (string, bool) {
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && equal(windows, k, name) {
			return v, true
		}
	}
	return "", false
}

func homeVar(windows bool) string {
	if windows {
		return "USERPROFILE"
	}
	return "HOME"
}
