//go:build !windows

package launch

import "syscall"

// Replace replaces graft with claude at argv0, started with argv and envv.
// It returns only if claude does not start.
func Replace(argv0 string, argv, envv []string) error {
	return syscall.Exec(argv0, argv, envv)
}
