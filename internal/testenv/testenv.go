// Package testenv lets the tests of graft run alike on Unix and on Windows:
// it sets the home directory the way each system reads it, and installs the
// test binary as the claude that graft starts. Only tests import it.
package testenv

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// SetHome makes dir the home directory for the rest of the test: HOME, which
// os.UserHomeDir reads on Unix, and on Windows also USERPROFILE, which it
// and Claude Code read there.
func SetHome(t testing.TB, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	}
}

// HomeEnv returns the variables that make dir the home directory of a child
// process, such as a graft that a test runs for real.
func HomeEnv(dir string) []string {
	env := []string{"HOME=" + dir}
	if runtime.GOOS == "windows" {
		env = append(env, "USERPROFILE="+dir)
	}
	return env
}

// ClaudeFile is the name graft asks PATH for: claude.exe on Windows, where
// a claude.cmd must not run.
func ClaudeFile() string {
	if runtime.GOOS == "windows" {
		return "claude.exe"
	}
	return "claude"
}

// InstallClaude puts this test binary into dir as claude and returns its
// path, for a test that runs graft for real and lets the binary play claude:
// a symbolic link named claude on Unix, and on Windows, where a link needs a
// privilege, a copy named claude.exe.
func InstallClaude(t testing.TB, dir string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ClaudeFile())
	if runtime.GOOS != "windows" {
		if err := os.Symlink(exe, path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if err := copyFile(exe, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
