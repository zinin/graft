package ui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graft", "config.yaml")
	s := NewStore(path)
	if data, v, err := s.Read(); data != nil || v != "" || err != nil {
		t.Fatalf("Read of no file = %q, %q, %v", data, v, err)
	}
	v, backup, err := s.Write("", []byte("a: 1\n"))
	if err != nil || backup != "" {
		t.Fatalf("Write = %q, %q, %v", v, backup, err)
	}
	if v != version([]byte("a: 1\n")) {
		t.Errorf("version %q", v)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, want 0600", info.Mode().Perm())
	}
	if info, _ := os.Stat(filepath.Dir(path)); info.Mode().Perm() != 0o700 {
		t.Errorf("directory mode %v, want 0700", info.Mode().Perm())
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a new file has a backup: %v", err)
	}
}

func TestStoreKeepsModeAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("old\n"), 0o640)
	s := NewStore(path)
	_, v, _ := s.Read()
	if _, backup, err := s.Write(v, []byte("new\n")); err != nil || backup != path+".bak" {
		t.Fatalf("Write = %q, %v", backup, err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Errorf("file %q", got)
	}
	if got, _ := os.ReadFile(path + ".bak"); string(got) != "old\n" {
		t.Errorf("backup %q", got)
	}
	for _, p := range []string{path, path + ".bak"} {
		if info, _ := os.Stat(p); info.Mode().Perm() != 0o640 {
			t.Errorf("%s mode %v, want 0640", p, info.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 2 {
		t.Errorf("left files behind: %v", entries)
	}
}

func TestStoreRefusesAStaleVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("old\n"), 0o600)
	s := NewStore(path)
	_, v, _ := s.Read()
	os.WriteFile(path, []byte("edited by hand\n"), 0o600)
	if _, _, err := s.Write(v, []byte("new\n")); !errors.Is(err, ErrConflict) {
		t.Errorf("Write = %v, want ErrConflict", err)
	}
	if _, _, err := s.Write("", []byte("new\n")); !errors.Is(err, ErrConflict) {
		t.Errorf("Write as a new file = %v, want ErrConflict", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "edited by hand\n" {
		t.Errorf("file %q", got)
	}
}

func TestStoreWritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "config.yaml")
	os.MkdirAll(filepath.Dir(target), 0o700)
	os.WriteFile(target, []byte("old\n"), 0o600)
	link := filepath.Join(dir, "config.yaml")
	os.Symlink(target, link)
	s := NewStore(link)
	_, v, _ := s.Read()
	if _, _, err := s.Write(v, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link is gone: %v, %v", info, err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new\n" {
		t.Errorf("target %q", got)
	}
	if got, _ := os.ReadFile(target + ".bak"); string(got) != "old\n" {
		t.Errorf("backup %q", got)
	}
}

func TestStoreRefusesABrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "config.yaml")
	os.Symlink(filepath.Join(dir, "missing.yaml"), link)
	if _, _, err := NewStore(link).Write("", []byte("a: 1\n")); err == nil {
		t.Error("Write through a broken symlink succeeded")
	}
}

func TestStoreVersionFollowsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	s := NewStore(path)
	if v, err := s.Version(); v != "" || err != nil {
		t.Fatalf("Version of no file = %q, %v", v, err)
	}
	os.WriteFile(path, []byte("a\n"), 0o600)
	v1, _ := s.Version()
	if v1 != version([]byte("a\n")) {
		t.Fatalf("version %q", v1)
	}
	os.WriteFile(path, []byte("bb\n"), 0o600)
	os.Chtimes(path, time.Now(), time.Now().Add(time.Second))
	if v2, _ := s.Version(); v2 != version([]byte("bb\n")) {
		t.Errorf("version after a change %q", v2)
	}
}
