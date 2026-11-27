package testenv

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestHomeEnv(t *testing.T) {
	want := []string{"HOME=/h"}
	if runtime.GOOS == "windows" {
		want = append(want, "USERPROFILE=/h")
	}
	if got := HomeEnv("/h"); !slices.Equal(got, want) {
		t.Errorf("HomeEnv = %q, want %q", got, want)
	}
}

func TestSetHome(t *testing.T) {
	dir := t.TempDir()
	SetHome(t, dir)
	if home, err := os.UserHomeDir(); err != nil || home != dir {
		t.Errorf("os.UserHomeDir = %q, %v; want %q", home, err, dir)
	}
}

func TestInstallClaude(t *testing.T) {
	path := InstallClaude(t, t.TempDir())
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	if filepath.Base(path) != name {
		t.Errorf("InstallClaude put %s, want %s", path, name)
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		t.Errorf("stat %s: %v", path, err)
	}
}
