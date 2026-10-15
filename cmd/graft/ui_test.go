package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a terminal that a test reads while graft ui writes to it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// uiRun is graft ui running in the background with a fake terminal,
// browser and signals.
type uiRun struct {
	out    *syncBuffer
	stop   chan os.Signal
	done   chan int
	opened []string // read only after done
}

func startUI(t *testing.T, f *fake, args ...string) *uiRun {
	t.Helper()
	r := &uiRun{out: &syncBuffer{}, stop: make(chan os.Signal, 1), done: make(chan int, 1)}
	f.sys.stdout = r.out
	f.sys.notify = func() (<-chan os.Signal, func()) { return r.stop, func() {} }
	f.sys.openBrowser = func(url string) { r.opened = append(r.opened, url) }
	go func() { r.done <- run(append(args, "ui"), f.sys) }()
	return r
}

// url waits for the address that graft ui prints.
func (r *uiRun) url(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, line := range strings.Split(r.out.String(), "\n") {
			if url, ok := strings.CutPrefix(line, "graft ui: open "); ok {
				return url
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no address in:\n%s", r.out.String())
	return ""
}

// finish stops graft ui and returns its exit code.
func (r *uiRun) finish(t *testing.T) int {
	t.Helper()
	r.stop <- os.Interrupt
	select {
	case code := <-r.done:
		return code
	case <-time.After(5 * time.Second):
		t.Fatal("graft ui did not stop")
		return -1
	}
}

// getConfig calls GET /api/config of the page at url.
func getConfig(t *testing.T, url string) map[string]any {
	t.Helper()
	base, token, _ := strings.Cut(url, "/#token=")
	req, _ := http.NewRequest("GET", base+"/api/config", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st map[string]any
	json.NewDecoder(resp.Body).Decode(&st)
	return st
}

func TestUI(t *testing.T) {
	f := newFake(t, "DISPLAY=:0")
	f.sys.goos = "linux"
	f.writeConfig(t)
	r := startUI(t, f)
	url := r.url(t)
	if st := getConfig(t, url); st["editable"] != true || st["exists"] != true {
		t.Errorf("config %v", st)
	}
	if code := r.finish(t); code != 0 {
		t.Errorf("code %d", code)
	}
	if len(r.opened) != 1 || r.opened[0] != url {
		t.Errorf("opened %v, want %s", r.opened, url)
	}
	if want := "graft ui: editing ~/.config/graft/config.yaml\n"; !strings.HasPrefix(r.out.String(), want) {
		t.Errorf("stdout %q", r.out.String())
	}
}

func TestUIWithoutConfig(t *testing.T) {
	f := newFake(t)
	r := startUI(t, f, "--config", filepath.Join(t.TempDir(), "config.yaml"))
	if st := getConfig(t, r.url(t)); st["exists"] != false || st["editable"] != true {
		t.Errorf("config %v", st)
	}
	r.finish(t)
	if len(r.opened) != 0 {
		t.Errorf("opened a browser without a display: %v", r.opened)
	}
}

func TestUIOverSSH(t *testing.T) {
	f := newFake(t, "DISPLAY=localhost:10.0", "SSH_CONNECTION=10.0.0.1 50000 10.0.0.2 22")
	f.sys.goos = "linux"
	r := startUI(t, f)
	r.url(t)
	r.finish(t)
	if len(r.opened) != 0 {
		t.Errorf("opened a browser over SSH: %v", r.opened)
	}
	if !strings.Contains(r.out.String(), "graft ui: from another machine: ssh -L ") {
		t.Errorf("stdout %q", r.out.String())
	}
}

func TestUIOnMacOS(t *testing.T) {
	f := newFake(t)
	f.sys.goos = "darwin"
	r := startUI(t, f)
	url := r.url(t)
	r.finish(t)
	if len(r.opened) != 1 || r.opened[0] != url {
		t.Errorf("opened %v", r.opened)
	}
}

func TestUITakesNoArguments(t *testing.T) {
	f := newFake(t)
	if code := run([]string{"ui", "extra"}, f.sys); code != 1 || !strings.Contains(f.stderr.String(), "ui takes no arguments") {
		t.Errorf("code %d, stderr %q", code, f.stderr.String())
	}
}
