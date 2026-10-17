package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

// previewConfig has a key in a file and a key from a command that leaves a
// mark when it runs.
func previewConfig(mark string) string {
	return `providers:
  ollama: {url: https://ollama.com, key: {file: keys/ollama}}
  local: {url: "http://127.0.0.1:8317", key: {command: "touch '` + mark + `'; echo sk"}}
models:
  glm: {provider: ollama, id: glm-5.3-flash, context: 1048576}
  sol: {provider: local, id: gpt-6.1-sol, context: 400000}
profiles:
  glm: {model: glm}
  mix: {model: opus, picker: [glm, sol]}
`
}

func previewOptions(t *testing.T) Options {
	return Options{
		Environ:  []string{"ANTHROPIC_API_KEY=sk-ant-leak", "PATH=/usr/bin:/bin"},
		Home:     t.TempDir(),
		Dir:      t.TempDir(),
		LookPath: func(name string) (string, error) { return "/opt/claude/bin/" + name, nil },
	}
}

// previewOf asks the API for the preview of profile in the draft of the file.
func (s *testAPI) previewOf(t *testing.T, profile string) previewResult {
	t.Helper()
	st := s.load(t)
	var res previewResult
	if code := s.call(t, "POST", "/api/preview", map[string]any{"version": st.Version, "draft": st.Draft, "profile": profile}, &res); code != 200 {
		t.Fatalf("POST /api/preview: %d", code)
	}
	return res
}

// TestPreview: the text of --dry-run, with each key as its source; no key is
// read.
func TestPreview(t *testing.T) {
	mark := filepath.Join(t.TempDir(), "ran")
	s := startWith(t, previewConfig(mark), previewOptions(t))
	file := "<file " + config.Tildify(filepath.Join(filepath.Dir(s.path), "keys", "ollama")) + ">"
	command := `<command "touch '` + mark + `'; echo sk">`
	tests := map[string][]string{
		"glm": {
			"profile glm: direct, provider ollama\n",
			"unset  ANTHROPIC_API_KEY\n",
			"set    ANTHROPIC_BASE_URL=https://ollama.com\n",
			"       ANTHROPIC_AUTH_TOKEN=" + file + "\n",
			"exec   /opt/claude/bin/claude --model glm-5.3-flash --settings '{",
			"settings\n{\n",
		},
		"mix": {
			"profile mix: router, providers ollama, local, subscription\n",
			"route  glm -> ollama glm-5.3-flash " + file + "\n",
			"       sol -> local gpt-6.1-sol " + command + "\n",
		},
	}
	for profile, lines := range tests {
		res := s.previewOf(t, profile)
		if res.Stale || len(res.Errors) > 0 || res.Failure != "" || len(res.Warnings) > 0 {
			t.Errorf("%s: %+v", profile, res)
		}
		for _, line := range lines {
			if !strings.Contains(res.Text, line) {
				t.Errorf("%s: the preview lacks %q:\n%s", profile, line, res.Text)
			}
		}
	}
	if _, err := os.Stat(mark); err == nil {
		t.Error("Preview ran a key command")
	}
}

func TestPreviewOfADraftWithErrors(t *testing.T) {
	s := startWith(t, previewConfig("/nonexistent/mark"), previewOptions(t))
	st := s.load(t)
	d := *st.Draft
	d.Models[0].Context = "5"
	var res previewResult
	s.call(t, "POST", "/api/preview", map[string]any{"version": st.Version, "draft": d, "profile": "glm"}, &res)
	if len(res.Errors) != 1 || res.Errors[0].Path != "models.glm.context" || res.Text != "" {
		t.Errorf("result %+v", res)
	}
	res = previewResult{}
	s.call(t, "POST", "/api/preview", map[string]any{"version": "old", "draft": st.Draft, "profile": "glm"}, &res)
	if !res.Stale || res.Text != "" {
		t.Errorf("a stale version: %+v", res)
	}
}

func TestPreviewFailures(t *testing.T) {
	broken := previewOptions(t)
	if err := os.MkdirAll(filepath.Join(broken.Dir, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken.Dir, ".claude", "settings.json"), []byte(`{"apiKeyHelper": "x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	unreadable := previewOptions(t)
	if err := os.MkdirAll(filepath.Join(unreadable.Dir, ".claude", "settings.local.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	noClaude := previewOptions(t)
	noClaude.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	tests := []struct {
		name    string
		opts    Options
		profile string
		failure string // substring; "" means none
		warning string // substring; "" means none
	}{
		{"settings that would override the key", broken, "glm", "Claude Code settings would override its address or key", ""},
		{"a settings file that cannot be read", unreadable, "glm", "", "settings.local.json"},
		{"no claude", noClaude, "glm", "claude not found in PATH", ""},
		{"no such profile", previewOptions(t), "nope", `no profile "nope"`, ""},
	}
	for _, tt := range tests {
		s := startWith(t, previewConfig("/nonexistent/mark"), tt.opts)
		res := s.previewOf(t, tt.profile)
		if (tt.failure == "") != (res.Failure == "") || !strings.Contains(res.Failure, tt.failure) {
			t.Errorf("%s: failure %q, want %q", tt.name, res.Failure, tt.failure)
		}
		if got := strings.Join(res.Warnings, "\n"); (tt.warning == "") != (got == "") || !strings.Contains(got, tt.warning) {
			t.Errorf("%s: warnings %q, want %q", tt.name, got, tt.warning)
		}
	}
}
