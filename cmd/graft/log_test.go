package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogPath(t *testing.T) {
	tests := []struct {
		home    string
		environ []string
		want    string // empty: an error
	}{
		{"/home/u", nil, "/home/u/.local/state/graft/graft.log"},
		{"/home/u", []string{"XDG_STATE_HOME=/state"}, "/state/graft/graft.log"},
		{"", []string{"XDG_STATE_HOME=/state"}, "/state/graft/graft.log"},
		{"", nil, ""},
	}
	for _, tt := range tests {
		got, err := logPath(system{home: tt.home, environ: tt.environ})
		if tt.want == "" {
			if err == nil {
				t.Errorf("logPath with no home and no XDG_STATE_HOME = %q, want an error", got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("logPath(%q, %q) = %q, %v; want %q", tt.home, tt.environ, got, err, tt.want)
		}
	}
}

func TestOpenLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "graft", "graft.log")
	for _, line := range []string{"one\n", "two\n"} {
		f, err := openLog(path)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(line)
		f.Close()
	}
	if got := readFile(t, path); got != "one\ntwo\n" {
		t.Errorf("log %q, want the lines of both sessions", got)
	}
	for _, p := range []string{filepath.Dir(path), path} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s has mode %o, want no access for others", p, perm)
		}
	}
}

func TestOpenLogMovesALargeLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graft.log")
	big := strings.Repeat("x", 10<<20+1)
	writeFile(t, path, big, 0o600)
	writeFile(t, path+".1", "older\n", 0o600)
	f, err := openLog(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got := readFile(t, path); got != "" {
		t.Errorf("the log holds %d bytes, want a new file", len(got))
	}
	if got := readFile(t, path+".1"); got != big {
		t.Errorf("graft.log.1 holds %d bytes, want the old log", len(got))
	}

	writeFile(t, path, strings.Repeat("x", 10<<20), 0o600)
	if f, err = openLog(path); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if info, _ := os.Stat(path); info.Size() != 10<<20 {
		t.Errorf("a log of exactly 10 MiB moved")
	}
}
