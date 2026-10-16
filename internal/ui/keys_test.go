package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/keys"
)

// checkKey asks the API for the state of a key source.
func (s *testAPI) checkKey(t *testing.T, kind, value string, run bool) keyState {
	t.Helper()
	var st keyState
	if code := s.call(t, "POST", "/api/key/check", map[string]any{"kind": kind, "value": value, "run": run}, &st); code != 200 {
		t.Fatalf("POST /api/key/check: %d", code)
	}
	return st
}

// checkKeyWithin is checkKey for a check that might wait for a key command
// or a key file: it gives up after d, so that such a wait fails the test
// instead of hanging it, and it returns a failure as an error, since t.Fatal
// must not run off the test's goroutine.
func (s *testAPI) checkKeyWithin(d time.Duration, kind, value string, run bool) (keyState, error) {
	var st keyState
	code, err := s.postWithin(d, "/api/key/check", map[string]any{"kind": kind, "value": value, "run": run}, &st)
	if err == nil && code != 200 {
		err = fmt.Errorf("POST /api/key/check: %d", code)
	}
	return st, err
}

// postWithin posts body to path and decodes the answer into out, as call
// does, but it gives up after d and returns a failure as an error: see
// checkKeyWithin.
func (s *testAPI) postWithin(d time.Duration, path string, body, out any) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	resp, err := (&http.Client{Timeout: d}).Post(s.srv.URL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, json.NewDecoder(resp.Body).Decode(out)
}

// writeKeyFor puts content into keys/name next to the config of s.
func writeKeyFor(t *testing.T, s *testAPI, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(s.path), "keys", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { // WriteFile's mode is filtered by umask
		t.Fatal(err)
	}
	return path
}

func TestKeyCheckFile(t *testing.T) {
	s := startAPI(t, "")
	good := writeKeyFor(t, s, "good", " sk-good \n", 0o600)
	open := writeKeyFor(t, s, "open", "sk-open\n", 0o644)
	two := writeKeyFor(t, s, "two", "sk-a\nsk-b\n", 0o600)
	none := filepath.Join(filepath.Dir(s.path), "keys", "none")
	tests := []struct {
		value string
		want  keyState
	}{
		{"keys/good", keyState{State: "ok", Path: config.Tildify(good), Fingerprint: keys.Fingerprint("sk-good")}},
		{good, keyState{State: "ok", Path: config.Tildify(good), Fingerprint: keys.Fingerprint("sk-good")}},
		{"keys//good", keyState{State: "ok", Path: config.Tildify(good), Fingerprint: keys.Fingerprint("sk-good")}},
		{"keys/none", keyState{State: "missing", Path: config.Tildify(none)}},
		{"keys/open", keyState{State: "ok", Path: config.Tildify(open), Fingerprint: keys.Fingerprint("sk-open"),
			Warning: "key file " + config.Tildify(open) + " is readable by other users: run chmod 600 on it"}},
		{"keys/two", keyState{State: "error", Path: config.Tildify(two),
			Error: "file " + config.Tildify(two) + ": the key must be one line without spaces"}},
		{" ", keyState{State: "error", Error: "must not be empty"}},
	}
	for _, tt := range tests {
		if got := s.checkKey(t, "file", tt.value, false); got != tt.want {
			t.Errorf("file %q: %+v, want %+v", tt.value, got, tt.want)
		}
	}
}

func TestKeyCheckEnv(t *testing.T) {
	s := startWith(t, "", Options{Environ: []string{"OLLAMA_API_KEY=sk-env", "EMPTY="}})
	tests := []struct {
		name string
		want keyState
	}{
		{"OLLAMA_API_KEY", keyState{State: "ok", Fingerprint: keys.Fingerprint("sk-env")}},
		{"MISSING", keyState{State: "unset"}},
		{"EMPTY", keyState{State: "error", Error: "env EMPTY: the key is empty"}},
	}
	for _, tt := range tests {
		if got := s.checkKey(t, "env", tt.name, false); got != tt.want {
			t.Errorf("env %s: %+v, want %+v", tt.name, got, tt.want)
		}
	}
	if got := s.checkKey(t, "", "x", false); got.State != "error" {
		t.Errorf("no kind: %+v, want an error", got)
	}
}

// TestKeyCommandRunsOnlyWhenAsked: a key command may ask for a password, so
// it runs only for Run, and its key is kept for what follows.
func TestKeyCommandRunsOnlyWhenAsked(t *testing.T) {
	s := startAPI(t, "")
	dir := t.TempDir()
	count, key := filepath.Join(dir, "count"), filepath.Join(dir, "key")
	if err := os.WriteFile(key, []byte("sk-cmd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := "echo run >> '" + count + "'; cat '" + key + "'"
	if st := s.checkKey(t, "command", cmd, false); st != (keyState{State: "not run"}) {
		t.Errorf("before Run: %+v", st)
	}
	if _, err := os.Stat(count); err == nil {
		t.Fatal("the command ran without Run")
	}
	want := keyState{State: "ok", Fingerprint: keys.Fingerprint("sk-cmd")}
	if st := s.checkKey(t, "command", cmd, true); st != want {
		t.Errorf("Run: %+v, want %+v", st, want)
	}
	if st := s.checkKey(t, "command", cmd, false); st != want {
		t.Errorf("after Run: %+v, want the kept key %+v", st, want)
	}
	if got := readFile(t, count); got != "run\n" {
		t.Errorf("the command ran %d times, want once", strings.Count(got, "run"))
	}
	// A run that fails drops the kept key.
	os.Remove(key)
	if st := s.checkKey(t, "command", cmd, true); st.State != "error" {
		t.Errorf("a failed run: %+v", st)
	}
	if st := s.checkKey(t, "command", cmd, false); st != (keyState{State: "not run"}) {
		t.Errorf("after a failed run: %+v, want no kept key", st)
	}
}

// TestKeyCommandsRunOneAtATime: two commands never ask for a password at
// once, and env, file and a check without Run do not wait for a command.
func TestKeyCommandsRunOneAtATime(t *testing.T) {
	s := startWith(t, "", Options{Environ: []string{"OLLAMA_API_KEY=sk-env"}})
	dir := t.TempDir()
	log, gate := filepath.Join(dir, "log"), filepath.Join(dir, "gate")
	// A test that stops early must still open the gate: the commands would
	// loop on it for ever, and the server, which closes once its requests
	// end, would hang the test. The cleanup closes the server too, so that
	// the commands end before their files go.
	t.Cleanup(func() {
		os.WriteFile(gate, nil, 0o600)
		s.srv.Close()
	})
	// Each command waits for the gate, so the first holds its turn until the
	// test opens it.
	cmd := func(n string) string {
		return "echo start >> '" + log + "'; while [ ! -e '" + gate + "' ]; do sleep 0.02; done; echo end >> '" + log + "'; echo sk-" + n
	}
	// Each run reports to the test, which alone may stop on a failure.
	type result struct {
		st  keyState
		err error
	}
	runs := map[string]chan result{}
	for _, n := range []string{"1", "2"} {
		ch := make(chan result, 1)
		runs[n] = ch
		go func() {
			st, err := s.checkKeyWithin(10*time.Second, "command", cmd(n), true)
			ch <- result{st, err}
		}()
	}
	waitFile(t, log, "start\n")
	switch st, err := s.checkKeyWithin(2*time.Second, "env", "OLLAMA_API_KEY", false); {
	case errors.Is(err, context.DeadlineExceeded):
		t.Error("env waited for a command")
	case err != nil || st.State != "ok":
		t.Errorf("env while a command waits: %+v %v", st, err)
	}
	switch st, err := s.checkKeyWithin(2*time.Second, "command", cmd("1"), false); {
	case errors.Is(err, context.DeadlineExceeded):
		t.Error("a check without Run waited for a command")
	case err != nil || st != (keyState{State: "not run"}):
		t.Errorf("a check without Run while a command waits: %+v %v", st, err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := readFile(t, log); got != "start\n" {
		t.Errorf("a second command started while the first waited:\n%s", got)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"1", "2"} {
		r := <-runs[n]
		if want := (keyState{State: "ok", Fingerprint: keys.Fingerprint("sk-" + n)}); r.err != nil || r.st != want {
			t.Errorf("the run of command %s: %+v %v, want %+v", n, r.st, r.err, want)
		}
	}
	if got := readFile(t, log); got != "start\nend\nstart\nend\n" {
		t.Errorf("the commands overlapped:\n%s", got)
	}
}

// waitFile waits until the file at path holds want.
func waitFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, _ := os.ReadFile(path); string(data) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never held %q", path, want)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestKeyWrite(t *testing.T) {
	s := startAPI(t, "")
	path := filepath.Join(filepath.Dir(s.path), "keys", "ollama")
	var got keyWritten
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "  sk-new \n"}, &got); code != 200 {
		t.Fatalf("write: %d", code)
	}
	if want := (keyWritten{Path: config.Tildify(path), Fingerprint: keys.Fingerprint("sk-new")}); got != want {
		t.Errorf("answer %+v, want %+v", got, want)
	}
	if content := readFile(t, path); content != "sk-new\n" {
		t.Errorf("file %q", content)
	}
	for p, mode := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != mode {
			t.Errorf("%s: mode %v, want %v (%v)", p, info.Mode().Perm(), mode, err)
		}
	}
	line := "graft ui: wrote " + config.Tildify(path) + " (sha256:" + keys.Fingerprint("sk-new") + ")\n"
	if s.log.String() != line {
		t.Errorf("terminal %q, want %q", s.log.String(), line)
	}

	// Another write needs replace, and the answer tells which key is there.
	var conflict keyConflict
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "sk-other"}, &conflict); code != 409 {
		t.Fatalf("write over a key: %d, want 409", code)
	}
	if want := (keyConflict{Error: config.Tildify(path) + " already holds a key", Fingerprint: keys.Fingerprint("sk-new")}); conflict != want {
		t.Errorf("conflict %+v, want %+v", conflict, want)
	}
	if s.log.String() != line {
		t.Errorf("terminal after a refused write %q, want %q", s.log.String(), line)
	}
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "sk-other", "replace": true}, nil); code != 200 {
		t.Fatalf("replace: %d", code)
	}
	if content := readFile(t, path); content != "sk-other\n" {
		t.Errorf("file after replace %q", content)
	}
	replaced := line + "graft ui: wrote " + config.Tildify(path) + " (sha256:" + keys.Fingerprint("sk-other") + ")\n"
	if s.log.String() != replaced {
		t.Errorf("terminal after replace %q, want %q", s.log.String(), replaced)
	}
	if strings.Contains(s.log.String(), "sk-new") || strings.Contains(s.log.String(), "sk-other") {
		t.Errorf("the terminal shows a key:\n%s", s.log.String())
	}
}

func TestKeyWriteRefusals(t *testing.T) {
	s := startAPI(t, "")
	tests := []struct {
		provider, key, msg string
	}{
		{"../x", "sk-1", "a provider name may contain lowercase letters, digits, '-' and '_'"},
		{"Ollama", "sk-1", "a provider name may contain lowercase letters, digits, '-' and '_'"},
		{"graft", "sk-1", `"graft" is reserved`},
		{"ollama", " \n", "the key is empty"},
		{"ollama", "sk-1 sk-2", "the key must be one line without spaces"},
		{"ollama", "\ufeffsk-1", "the key contains U+FEFF, which is not a printable ASCII character"},
	}
	for _, tt := range tests {
		var body errorBody
		if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": tt.provider, "key": tt.key}, &body); code != 422 || body.Error != tt.msg {
			t.Errorf("%q %q: %d %q, want 422 %q", tt.provider, tt.key, code, body.Error, tt.msg)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(s.path))
	if len(entries) != 0 {
		t.Errorf("a refused write left %v", entries)
	}
	if s.log.String() != "" {
		t.Errorf("a refused write printed %q", s.log.String())
	}
}

func TestKeyWriteMakesTheDirectories(t *testing.T) {
	path := filepath.Join(tempDir(t), "new", "config.yaml")
	mux := http.NewServeMux()
	newAPI(Options{Path: path, Log: &lockedBuffer{}}).routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	s := &testAPI{srv: srv, path: path}
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "sk-1"}, nil); code != 200 {
		t.Fatalf("write: %d", code)
	}
	for _, dir := range []string{filepath.Dir(path), filepath.Join(filepath.Dir(path), "keys")} {
		if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v %v, want mode 0700", dir, info, err)
		}
	}
}

func TestKeyWriteFailure(t *testing.T) {
	s := startAPI(t, "")
	path := filepath.Join(filepath.Dir(s.path), "keys", "ollama")
	if err := os.MkdirAll(filepath.Join(path, "inside"), 0o700); err != nil {
		t.Fatal(err)
	}
	var body errorBody
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "sk-1", "replace": true}, &body); code != 500 {
		t.Fatalf("write over a directory: %d, want 500", code)
	}
	if !strings.HasPrefix(s.log.String(), "graft ui: cannot write "+config.Tildify(path)+": ") {
		t.Errorf("terminal %q", s.log.String())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("the failed write left %v", entries)
	}
}
