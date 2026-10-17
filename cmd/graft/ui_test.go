package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zinin/graft/internal/keys"
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
	if st := getConfig(t, url); st["editable"] != true || st["exists"] != true || st["explicit"] != false {
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
	f.sys.goos = "linux" // an unset goos would refuse the browser whatever the display
	r := startUI(t, f, "--config", filepath.Join(t.TempDir(), "config.yaml"))
	if st := getConfig(t, r.url(t)); st["exists"] != false || st["editable"] != true || st["explicit"] != true {
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

// TestUICanBrowse pins the one gate before graft ui starts another program
// with the session token in its arguments: macOS, or Linux with a display,
// and never over SSH.
func TestUICanBrowse(t *testing.T) {
	const ssh = "SSH_CONNECTION=10.0.0.1 50000 10.0.0.2 22"
	tests := []struct {
		name    string
		goos    string
		environ []string
		want    bool
	}{
		{"macOS", "darwin", nil, true},
		{"macOS over SSH", "darwin", []string{ssh}, false},
		{"Linux without a display", "linux", nil, false},
		{"Linux with empty display variables", "linux", []string{"DISPLAY=", "WAYLAND_DISPLAY="}, false},
		{"Linux with X11", "linux", []string{"DISPLAY=:0"}, true},
		{"Linux with Wayland only", "linux", []string{"WAYLAND_DISPLAY=wayland-0"}, true},
		{"Linux over SSH with X11 forwarding", "linux", []string{"DISPLAY=localhost:10.0", ssh}, false},
		{"another system with a display", "freebsd", []string{"DISPLAY=:0"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, tt.environ...)
			f.sys.goos = tt.goos
			if got := canBrowse(f.sys); got != tt.want {
				t.Errorf("canBrowse = %v, want %v", got, tt.want)
			}
		})
	}
}

// callAPI sends a request to the API of the page at url and decodes the
// JSON answer into out.
func callAPI(t *testing.T, url, method, path string, body, out any) int {
	t.Helper()
	base, token, _ := strings.Cut(url, "/#token=")
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, base+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp.StatusCode
}

// TestPreviewMatchesDryRun: Preview prints what --dry-run prints, with each
// key shown by its source instead of its source and fingerprint, and graft
// ui reads keys from its own environment. Preview warns of the Claude Code
// settings files that --dry-run warns of.
func TestPreviewMatchesDryRun(t *testing.T) {
	f := newFake(t, "OLLAMA_API_KEY=sk-ollama", "ANTHROPIC_API_KEY=sk-ant-leak")
	f.writeConfig(t)
	// Project settings that cannot be read, in the directory both start in:
	// only a Preview that checks that directory, and not the home, warns of
	// them as --dry-run does.
	if err := os.MkdirAll(filepath.Join(f.sys.cwd, ".claude", "settings.local.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	fingerprint := regexp.MustCompile(`, sha256:[0-9a-f]{12}>`)
	profiles := []string{"glm-proxy", "mix"}
	want := map[string]string{}
	warned := map[string][]string{}
	for _, profile := range profiles {
		f.stdout.Reset()
		f.stderr.Reset()
		if code := run([]string{"--dry-run", profile}, f.sys); code != 0 {
			t.Fatalf("--dry-run %s: code %d, stderr %s", profile, code, f.stderr.String())
		}
		want[profile] = fingerprint.ReplaceAllString(f.stdout.String(), ">")
		for _, line := range strings.Split(f.stderr.String(), "\n") {
			if w, ok := strings.CutPrefix(line, "graft: warning: "); ok {
				warned[profile] = append(warned[profile], w)
			}
		}
		if !slices.ContainsFunc(warned[profile], func(w string) bool { return strings.Contains(w, "settings.local.json") }) {
			t.Fatalf("--dry-run %s does not warn of the settings in its directory:\n%s", profile, f.stderr.String())
		}
	}
	r := startUI(t, f)
	url := r.url(t)
	var st struct {
		Version string         `json:"version"`
		Draft   map[string]any `json:"draft"`
	}
	callAPI(t, url, "GET", "/api/config", nil, &st)
	for _, profile := range profiles {
		var res struct {
			Text, Failure string
			Warnings      []string
		}
		callAPI(t, url, "POST", "/api/preview", map[string]any{"version": st.Version, "draft": st.Draft, "profile": profile}, &res)
		if res.Text != want[profile] || res.Failure != "" {
			t.Errorf("Preview of %s:\n%s%s\nwant what --dry-run prints:\n%s", profile, res.Text, res.Failure, want[profile])
		}
		if !slices.Equal(res.Warnings, warned[profile]) {
			t.Errorf("the warnings of Preview of %s: %q, want those of --dry-run: %q", profile, res.Warnings, warned[profile])
		}
	}
	var key struct {
		State, Fingerprint string
	}
	callAPI(t, url, "POST", "/api/key/check", map[string]any{"kind": "env", "value": "OLLAMA_API_KEY"}, &key)
	if key.State != "ok" || key.Fingerprint != keys.Fingerprint("sk-ollama") {
		t.Errorf("the key of env OLLAMA_API_KEY: %+v", key)
	}
	r.finish(t)
}
