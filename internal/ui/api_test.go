package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/zinin/graft/internal/edit"
)

const testConfig = `# my config
providers:
  ollama:
    url: https://ollama.com
    key: { env: OLLAMA_API_KEY }
models:
  glm: { provider: ollama, id: glm-5.3-flash, context: 1048576 } # the big one
profiles:
  glm: { model: glm }
  mix: { model: opus, picker: [glm] }
`

// lockedBuffer is a terminal that tests can read while the server writes.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// testAPI serves the API of a config file in a temporary directory, without
// the guards of Server; content "" means no file.
type testAPI struct {
	srv  *httptest.Server
	path string
	log  *lockedBuffer
}

func startAPI(t *testing.T, content string) *testAPI {
	t.Helper()
	path := filepath.Join(tempDir(t), "config.yaml")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := &lockedBuffer{}
	mux := http.NewServeMux()
	(&api{path: path, store: NewStore(path), log: log}).routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &testAPI{srv, path, log}
}

// call sends a request and decodes the JSON answer into out.
func (s *testAPI) call(t *testing.T, method, path string, body, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, s.srv.URL+path, r)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
	}
	return resp.StatusCode
}

func (s *testAPI) load(t *testing.T) fileState {
	t.Helper()
	var st fileState
	if code := s.call(t, "GET", "/api/config", nil, &st); code != 200 {
		t.Fatalf("GET /api/config: %d", code)
	}
	return st
}

func TestConfigOfAFile(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	if !st.Exists || !st.Editable || st.Version != version([]byte(testConfig)) || st.Draft == nil {
		t.Fatalf("state %+v", st)
	}
	if st.Path != s.path || len(st.Draft.Profiles) != 2 || st.Draft.Models[0].ID != "glm-5.3-flash" {
		t.Errorf("state %+v", st)
	}
}

func TestConfigWithoutAFile(t *testing.T) {
	s := startAPI(t, "")
	st := s.load(t)
	if st.Exists || !st.Editable || st.Version != "" || st.Draft == nil || len(st.Draft.Profiles) != 0 {
		t.Errorf("state %+v", st)
	}
}

func TestConfigTheFormsCannotShow(t *testing.T) {
	s := startAPI(t, "profiles:\n  p: { model: opus, picker: glm }\n")
	st := s.load(t)
	if st.Editable || st.Draft != nil || len(st.Problems) != 1 {
		t.Fatalf("state %+v", st)
	}
	if p := st.Problems[0]; p.Line != 2 || p.Path != "profiles.p.picker" || p.Msg != "must be a list" {
		t.Errorf("problem %+v", p)
	}
}

func TestValidate(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	var res checkResult
	s.call(t, "POST", "/api/validate", draftRequest{st.Version, *st.Draft}, &res)
	if res.Stale || len(res.Errors) != 0 {
		t.Fatalf("result %+v", res)
	}
	if got := res.Profiles["mix"]; got.Mode != "router" || !got.Subscription || len(got.Providers) != 1 {
		t.Errorf("mix %+v", got)
	}
	d := *st.Draft
	d.Profiles[1].Picker = []string{"glm", "nope"}
	res = checkResult{}
	s.call(t, "POST", "/api/validate", draftRequest{st.Version, d}, &res)
	if len(res.Errors) != 1 || res.Errors[0].Path != "profiles.mix.picker[1]" || len(res.Profiles) != 0 {
		t.Errorf("result %+v", res)
	}
	d.Profiles[1].Settings = "a: ["
	res = checkResult{}
	s.call(t, "POST", "/api/validate", draftRequest{st.Version, d}, &res)
	if len(res.Errors) != 1 || res.Errors[0].Path != "profiles.mix.settings" {
		t.Errorf("result %+v", res)
	}
	res = checkResult{}
	s.call(t, "POST", "/api/validate", draftRequest{"0123", *st.Draft}, &res)
	if !res.Stale {
		t.Errorf("result %+v, want stale", res)
	}
}

func TestSave(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	d := *st.Draft
	d.Profiles[0].Effort = "high"
	var saved savedFile
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, d}, &saved); code != 200 {
		t.Fatalf("save: %d", code)
	}
	data, _ := os.ReadFile(s.path)
	if saved.Version != version(data) || !edit.Same(saved.Draft, d) {
		t.Errorf("saved %+v", saved)
	}
	for _, want := range []string{"# my config", "# the big one", "glm: {model: glm, effort: high}"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("want %q in:\n%s", want, data)
		}
	}
	if backup, _ := os.ReadFile(s.path + ".bak"); string(backup) != testConfig {
		t.Errorf("backup:\n%s", backup)
	}
	if want := "graft ui: saved " + s.path + " (previous version in " + s.path + ".bak)\n"; s.log.String() != want {
		t.Errorf("log %q, want %q", s.log.String(), want)
	}
	// The page saves again from the version it got.
	d = saved.Draft
	d.Profiles[0].Effort = ""
	if code := s.call(t, "POST", "/api/save", draftRequest{saved.Version, d}, &saved); code != 200 {
		t.Errorf("second save: %d", code)
	}
}

func TestSaveRefusals(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	var body map[string]any
	if code := s.call(t, "POST", "/api/save", draftRequest{"0123", *st.Draft}, &body); code != 409 {
		t.Errorf("stale save: %d %v", code, body)
	}
	d := *st.Draft
	d.Profiles[0].Model = "nope"
	var res checkResult
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, d}, &res); code != 422 || len(res.Errors) != 1 {
		t.Errorf("invalid save: %d %+v", code, res)
	}
	if data, _ := os.ReadFile(s.path); string(data) != testConfig {
		t.Errorf("the file changed:\n%s", data)
	}
}

// TestSaveStopsWhenTheFileDiffers: a fault of the merge, which writes
// something else than the page shows, must not reach the disk.
func TestSaveStopsWhenTheFileDiffers(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	applyDraft = func(data []byte, d edit.Draft) ([]byte, error) {
		d.Profiles = slices.Clone(d.Profiles)
		d.Profiles[0].Effort = "low"
		return edit.Apply(data, d)
	}
	t.Cleanup(func() { applyDraft = edit.Apply })
	var body errorBody
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, *st.Draft}, &body); code != 500 {
		t.Fatalf("save: %d %+v", code, body)
	}
	if data, _ := os.ReadFile(s.path); string(data) != testConfig {
		t.Errorf("the file changed:\n%s", data)
	}
	if !strings.Contains(s.log.String(), "did not come out as the page showed it") {
		t.Errorf("log %q", s.log.String())
	}
}

func TestSaveCreatesTheFile(t *testing.T) {
	s := startAPI(t, "")
	st := s.load(t)
	d := *st.Draft
	d.Profiles = append(d.Profiles, edit.Profile{Name: "plain", Model: "opus"})
	var saved savedFile
	if code := s.call(t, "POST", "/api/save", draftRequest{"", d}, &saved); code != 200 {
		t.Fatalf("save: %d", code)
	}
	if data, _ := os.ReadFile(s.path); string(data) != "profiles:\n  plain:\n    model: opus\n" {
		t.Errorf("file:\n%s", data)
	}
	if want := "graft ui: created " + s.path + "\n"; s.log.String() != want {
		t.Errorf("log %q", s.log.String())
	}
}

func TestBadJSON(t *testing.T) {
	s := startAPI(t, testConfig)
	resp, err := http.Post(s.srv.URL+"/api/validate", "application/json", strings.NewReader("{"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("bad JSON: %d", resp.StatusCode)
	}
}

func TestTwoTabsSave(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	first, second := *st.Draft, *st.Draft
	first.Profiles = slices.Clone(first.Profiles)
	first.Profiles[0].Effort = "high"
	second.Profiles = slices.Clone(second.Profiles)
	second.Profiles[0].Effort = "low"
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, first}, nil); code != 200 {
		t.Fatalf("first save: %d", code)
	}
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, second}, nil); code != 409 {
		t.Errorf("second save from the same version: %d, want 409", code)
	}
	if data, _ := os.ReadFile(s.path); !strings.Contains(string(data), "effort: high") {
		t.Errorf("file:\n%s", data)
	}
}
