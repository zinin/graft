package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLogPath(t *testing.T) {
	tests := []struct {
		home    string
		environ []string
		want    string // empty: an error
	}{
		{"/home/u", nil, filepath.FromSlash("/home/u/.local/state/graft/graft.log")},
		{"/home/u", []string{"XDG_STATE_HOME=/state"}, filepath.FromSlash("/state/graft/graft.log")},
		{"", []string{"XDG_STATE_HOME=/state"}, filepath.FromSlash("/state/graft/graft.log")},
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
	if runtime.GOOS == "windows" {
		return // no mode bits: the log takes the access list of its folder
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

// TestOpenLogKeepsWritingWhenTheMoveFails: on Windows a log that another
// session holds open cannot be renamed; the session then writes on in the
// log rather than keep none.
func TestOpenLogKeepsWritingWhenTheMoveFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graft", "graft.log")
	writeFile(t, path, strings.Repeat("x", maxLog+1), 0o600)
	// A directory with a file in it cannot be replaced by a rename, on any system.
	writeFile(t, filepath.Join(path+".1", "inside"), "", 0o600)
	f, err := openLog(path)
	if err != nil {
		t.Fatalf("openLog: %v", err)
	}
	f.WriteString("new\n")
	f.Close()
	if got := readFile(t, path); !strings.HasSuffix(got, "xnew\n") {
		t.Errorf("the log does not end with the new line")
	}
	if info, err := os.Stat(path + ".1"); err != nil || !info.IsDir() {
		t.Errorf("%s.1 changed: %v", path, err)
	}
}
