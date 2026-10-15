package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestOpenBrowserStartsASessionOfItsOwn: the Ctrl+C that stops graft ui goes
// to the whole foreground process group of the terminal, and a browser that
// xdg-open has to start would die with it. So xdg-open runs in a session of
// its own, as its leader, and so do the processes it starts.
func TestOpenBrowserStartsASessionOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "recorded")
	// A fake xdg-open that records its pid, its session and its argument. It
	// takes the session from /proc, with shell built-ins only, since PATH is
	// just this directory: the fields after the command name in
	// /proc/PID/stat are the state, the parent, the group and the session.
	script := `#!/bin/sh
url=$1
read -r stat < /proc/$$/stat
set -- ${stat##*) }
echo "$$ $4 $url" > "` + out + `"
`
	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	const url = "http://127.0.0.1:1/#token=abc"
	openBrowser(url)

	var pid, sid int
	var arg string
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, _ := os.ReadFile(out)
		if n, _ := fmt.Sscanf(strings.TrimSpace(string(data)), "%d %d %s", &pid, &sid, &arg); n == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("xdg-open did not record anything: %q", data)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if arg != url {
		t.Errorf("xdg-open got %q, want %q", arg, url)
	}
	own, _, errno := syscall.Syscall(syscall.SYS_GETSID, 0, 0, 0)
	if errno != 0 {
		t.Fatal(errno)
	}
	if sid == int(own) {
		t.Errorf("xdg-open runs in the session of graft (%d): Ctrl+C would reach the browser", sid)
	}
	if sid != pid {
		t.Errorf("xdg-open (pid %d) is in session %d, not leader of its own", pid, sid)
	}
}
