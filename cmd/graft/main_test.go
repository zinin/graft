package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/zinin/graft/internal/keys"
)

func TestMain(m *testing.M) {
	// TestRouterProfileRuns runs this test binary as claude, behind graft.
	if filepath.Base(os.Args[0]) == "claude" {
		os.Exit(fakeClaude())
	}
	// TestExecReplacesProcess and TestRouterProfileRuns run this test binary
	// as graft itself.
	if os.Getenv("GRAFT_TEST_RUN_MAIN") == "1" {
		main()
	}
	os.Exit(m.Run())
}

// fakeClaude plays claude behind the router: it sends one request for the
// model glm and exits with 5. It leaves the answer in $FAKE_OUT/answer, and
// its NO_PROXY and no_proxy in $FAKE_OUT/no_proxy: Go never sends a request
// for 127.0.0.1 through a proxy, but Claude Code does.
func fakeClaude() int {
	out := os.Getenv("FAKE_OUT")
	os.WriteFile(filepath.Join(out, "no_proxy"), []byte(os.Getenv("NO_PROXY")+" "+os.Getenv("no_proxy")), 0o600)
	if model := os.Getenv("FAKE_SAVE_MODEL"); model != "" {
		// As Enter in /model does, where claude keeps its user settings.
		dir := os.Getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			dir = filepath.Join(os.Getenv("HOME"), ".claude")
		}
		os.MkdirAll(dir, 0o700)
		os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"model": "`+model+`"}`), 0o600)
	}
	name, value, _ := strings.Cut(os.Getenv("ANTHROPIC_CUSTOM_HEADERS"), ": ")
	req, _ := http.NewRequest(http.MethodPost, os.Getenv("ANTHROPIC_BASE_URL")+"/v1/messages", strings.NewReader(`{"model":"glm"}`))
	req.Header.Set(name, value)
	answer := ""
	if resp, err := http.DefaultClient.Do(req); err != nil {
		answer = err.Error()
	} else {
		body, _ := io.ReadAll(resp.Body)
		answer = fmt.Sprintf("%d %s", resp.StatusCode, body)
	}
	os.WriteFile(filepath.Join(out, "answer"), []byte(answer), 0o600)
	return 5
}

const testConfig = `
providers:
  cliproxy:
    url: http://127.0.0.1:8317
    key: { file: keys/cliproxy }
  ollama:
    url: https://ollama.com
    key: { env: OLLAMA_API_KEY }
models:
  glm-proxy: { provider: cliproxy, id: glm-5.3-flash, context: 524288 }
  glm:       { provider: ollama,   id: glm-5.3-flash, context: 1048576 }
defaults:
  effort: max
profiles:
  glm-proxy:
    model: glm-proxy
    settings: { autoCompactEnabled: true }
  glm: { model: glm }
  mix: { model: opus, picker: [glm] }
`

const cliproxyKey = "sk-cliproxy-secret"

type execCall struct {
	path       string
	argv, envv []string
}

// fake is a system with a temporary home and recorders in place of the
// terminal and of exec.
type fake struct {
	sys            system
	stdout, stderr strings.Builder
	exec           *execCall
}

func newFake(t *testing.T, environ ...string) *fake {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home) // config.Tildify reads it
	f := &fake{}
	f.sys = system{
		stdin:    strings.NewReader(""),
		stdout:   &f.stdout,
		stderr:   &f.stderr,
		environ:  append([]string{"HOME=" + home, "PATH=/usr/bin:/bin"}, environ...),
		home:     home,
		cwd:      t.TempDir(),
		lookPath: func(name string) (string, error) { return "/opt/claude/bin/" + name, nil },
		exec: func(path string, argv, envv []string) error {
			f.exec = &execCall{path, argv, envv}
			return nil
		},
		claudeVersion: func(string) (string, error) { return "2.1.296", nil },
	}
	return f
}

// writeConfig puts testConfig and the cliproxy key into ~/.config/graft.
func (f *fake) writeConfig(t *testing.T) {
	t.Helper()
	dir := filepath.Join(f.sys.home, ".config", "graft")
	writeFile(t, filepath.Join(dir, "config.yaml"), testConfig, 0o600)
	writeFile(t, filepath.Join(dir, "keys", "cliproxy"), cliproxyKey+"\n", 0o600)
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUsageAndVersion(t *testing.T) {
	tests := []struct {
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{nil, 1, "", "usage:"},
		{[]string{"--help"}, 0, "usage:", ""},
		{[]string{"-h"}, 0, "usage:", ""},
		{[]string{"help"}, 0, "usage:", ""},
		{[]string{"version"}, 0, "graft dev\n", ""},
		{[]string{"--version"}, 0, "graft dev\n", ""},
		{[]string{"--bogus"}, 1, "", "flag provided but not defined: -bogus"},
		{[]string{"list", "extra"}, 1, "", "list takes no arguments"},
		{[]string{"help", "extra"}, 1, "", "help takes no arguments"},
		{[]string{"version", "extra"}, 1, "", "version takes no arguments"},
	}
	for _, tt := range tests {
		f := newFake(t)
		if code := run(tt.args, f.sys); code != tt.code {
			t.Errorf("run(%q) = %d, want %d", tt.args, code, tt.code)
		}
		if !strings.Contains(f.stdout.String(), tt.stdout) || (tt.stdout == "" && f.stdout.Len() > 0) {
			t.Errorf("run(%q) stdout %q, want %q", tt.args, f.stdout.String(), tt.stdout)
		}
		if !strings.Contains(f.stderr.String(), tt.stderr) || (tt.stderr == "" && f.stderr.Len() > 0) {
			t.Errorf("run(%q) stderr %q, want %q", tt.args, f.stderr.String(), tt.stderr)
		}
	}
}

func TestConfigPath(t *testing.T) {
	tests := []struct {
		home    string
		flag    string
		environ []string
		want    string // empty: an error
	}{
		{"/home/u", "", nil, "/home/u/.config/graft/config.yaml"},
		{"/home/u", "", []string{"XDG_CONFIG_HOME=/xdg"}, "/xdg/graft/config.yaml"},
		{"/home/u", "", []string{"XDG_CONFIG_HOME=/xdg", "GRAFT_CONFIG=~/g.yaml"}, "/home/u/g.yaml"},
		{"/home/u", "/etc/graft.yaml", []string{"GRAFT_CONFIG=/other.yaml"}, "/etc/graft.yaml"},
		{"", "/etc/graft.yaml", nil, "/etc/graft.yaml"},
		{"", "", []string{"XDG_CONFIG_HOME=/xdg"}, "/xdg/graft/config.yaml"},
		{"", "", nil, ""},
		{"", "~/graft.yaml", nil, ""},
	}
	for _, tt := range tests {
		got, err := configPath(tt.flag, system{home: tt.home, environ: tt.environ})
		if tt.want == "" {
			if err == nil || !strings.Contains(err.Error(), "cannot find the home directory") {
				t.Errorf("configPath(%q, %q) with no home = %q, %v; want an error", tt.flag, tt.environ, got, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("configPath(%q, %q) = %q, %v; want %q", tt.flag, tt.environ, got, err, tt.want)
		}
	}
}

func TestMissingConfig(t *testing.T) {
	f := newFake(t)
	if code := run([]string{"list"}, f.sys); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
	if want := "graft: no config at ~/.config/graft/config.yaml"; !strings.Contains(f.stderr.String(), want) {
		t.Errorf("stderr %q, want %q", f.stderr.String(), want)
	}
}

func TestConfigErrorsArePrintedOnePerLine(t *testing.T) {
	f := newFake(t)
	writeFile(t, filepath.Join(f.sys.home, ".config", "graft", "config.yaml"),
		"profiles:\n  a: { model: nope }\n  b: { model: opus, effort: huge }\n", 0o600)
	if code := run([]string{"list"}, f.sys); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
	lines := strings.Split(strings.TrimSpace(f.stderr.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `:2: profiles.a.model: unknown model "nope"`) ||
		!strings.Contains(lines[1], ":3: profiles.b.effort: must be one of") {
		t.Errorf("stderr:\n%s", f.stderr.String())
	}
}

func TestList(t *testing.T) {
	f := newFake(t)
	f.writeConfig(t)
	if code := run([]string{"list"}, f.sys); code != 0 {
		t.Fatalf("code %d, stderr %s", code, f.stderr.String())
	}
	want := `PROFILE    MODE    MODEL      SUBSCRIPTION  PROVIDERS
glm-proxy  direct  glm-proxy  no            cliproxy
glm        direct  glm        no            ollama
mix        router  opus       yes           ollama
`
	if f.stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", f.stdout.String(), want)
	}
}

func TestStart(t *testing.T) {
	f := newFake(t, "ANTHROPIC_API_KEY=sk-ant-leak")
	f.writeConfig(t)
	if code := run([]string{"glm-proxy", "--resume"}, f.sys); code != 0 {
		t.Fatalf("code %d, stderr %s", code, f.stderr.String())
	}
	if f.exec == nil {
		t.Fatal("claude was not started")
	}
	if f.exec.path != "/opt/claude/bin/claude" || f.exec.argv[0] != "claude" || f.exec.argv[len(f.exec.argv)-1] != "--resume" {
		t.Errorf("exec %s %q", f.exec.path, f.exec.argv)
	}
	env := strings.Join(f.exec.envv, "\n") + "\n"
	for _, want := range []string{"ANTHROPIC_AUTH_TOKEN=" + cliproxyKey + "\n", "ANTHROPIC_BASE_URL=http://127.0.0.1:8317\n"} {
		if !strings.Contains(env, want) {
			t.Errorf("environment lacks %q", want)
		}
	}
	if strings.Contains(env, "ANTHROPIC_API_KEY=") {
		t.Error("ANTHROPIC_API_KEY from the shell reached claude")
	}
	if strings.Contains(strings.Join(f.exec.argv, " "), cliproxyKey) {
		t.Error("the key is on the command line")
	}
}

func TestStartReadsKeyFromEnv(t *testing.T) {
	f := newFake(t, "OLLAMA_API_KEY=sk-ollama")
	f.writeConfig(t)
	if code := run([]string{"glm"}, f.sys); code != 0 {
		t.Fatalf("code %d, stderr %s", code, f.stderr.String())
	}
	if !strings.Contains(strings.Join(f.exec.envv, "\n"), "ANTHROPIC_AUTH_TOKEN=sk-ollama") {
		t.Error("claude did not get the key from OLLAMA_API_KEY")
	}
}

func TestStartErrors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"glm"}, "graft: provider ollama: env OLLAMA_API_KEY is not set"},
		{[]string{"mix"}, "graft: provider ollama: env OLLAMA_API_KEY is not set"},
		{[]string{"nope"}, `graft: no profile "nope" in ~/.config/graft/config.yaml`},
		{[]string{"glm-proxy", "--settings", "x.json"}, "graft: --settings is managed by graft"},
	}
	for _, tt := range tests {
		f := newFake(t)
		f.writeConfig(t)
		if code := run(tt.args, f.sys); code != 1 {
			t.Errorf("run(%q) = %d, want 1", tt.args, code)
		}
		if !strings.Contains(f.stderr.String(), tt.want) {
			t.Errorf("run(%q) stderr %q, want %q", tt.args, f.stderr.String(), tt.want)
		}
		if f.exec != nil {
			t.Errorf("run(%q) started claude", tt.args)
		}
	}
}

func TestStartRefusesConflictingClaudeSettings(t *testing.T) {
	f := newFake(t)
	f.writeConfig(t)
	writeFile(t, filepath.Join(f.sys.home, ".claude", "settings.json"), `{"env": {"ANTHROPIC_BASE_URL": "https://gateway.example"}}`, 0o600)
	if code := run([]string{"glm-proxy"}, f.sys); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
	if !strings.Contains(f.stderr.String(), "~/.claude/settings.json: env.ANTHROPIC_BASE_URL") {
		t.Errorf("stderr %q", f.stderr.String())
	}
}

// TestStartChecksProfileClaudeConfigDir: a profile's own CLAUDE_CONFIG_DIR is
// the directory claude will read its user settings from, so graft checks
// the settings there.
func TestStartChecksProfileClaudeConfigDir(t *testing.T) {
	f := newFake(t)
	f.writeConfig(t)
	dir := filepath.Join(t.TempDir(), "claude-glm")
	writeFile(t, filepath.Join(dir, "settings.json"), `{"env": {"ANTHROPIC_BASE_URL": "https://gateway.example"}}`, 0o600)
	writeFile(t, filepath.Join(f.sys.home, ".config", "graft", "config.yaml"),
		testConfig+"  isolated:\n    model: glm-proxy\n    env: { CLAUDE_CONFIG_DIR: "+dir+" }\n", 0o600)
	if code := run([]string{"isolated"}, f.sys); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
	if want := filepath.Join(dir, "settings.json") + ": env.ANTHROPIC_BASE_URL"; !strings.Contains(f.stderr.String(), want) {
		t.Errorf("stderr %q, want %q", f.stderr.String(), want)
	}
}

// TestStartWithoutHome: without a home directory graft cannot find the user
// settings that claude will read, so it does not start claude.
func TestStartWithoutHome(t *testing.T) {
	f := newFake(t)
	f.writeConfig(t)
	cfgPath := filepath.Join(f.sys.home, ".config", "graft", "config.yaml")
	f.sys.home = ""
	if code := run([]string{"--config", cfgPath, "glm-proxy"}, f.sys); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
	if !strings.Contains(f.stderr.String(), "cannot find the home directory") {
		t.Errorf("stderr %q", f.stderr.String())
	}
	if f.exec != nil {
		t.Error("claude was started")
	}
}

// TestSubscriptionProfileStartsWithoutHome: Claude Code's settings files
// matter only to a profile with external models, so a profile on the
// subscription alone starts without a home directory.
func TestSubscriptionProfileStartsWithoutHome(t *testing.T) {
	f := newFake(t)
	cfgPath := filepath.Join(f.sys.home, ".config", "graft", "config.yaml")
	writeFile(t, cfgPath, testConfig+"  plain: { model: opus }\n", 0o600)
	f.sys.home = ""
	f.sys.environ = []string{"PATH=/usr/bin:/bin"}
	if code := run([]string{"--config", cfgPath, "plain"}, f.sys); code != 0 {
		t.Fatalf("code %d, stderr %s", code, f.stderr.String())
	}
	if f.exec == nil {
		t.Error("claude was not started")
	}
}

func TestDryRun(t *testing.T) {
	f := newFake(t, "ANTHROPIC_API_KEY=sk-ant-leak")
	f.writeConfig(t)
	if code := run([]string{"--dry-run", "glm-proxy", "--resume"}, f.sys); code != 0 {
		t.Fatalf("code %d, stderr %s", code, f.stderr.String())
	}
	if f.exec != nil {
		t.Error("--dry-run started claude")
	}
	out := f.stdout.String()
	for _, want := range []string{
		"profile glm-proxy: direct, provider cliproxy\n",
		"unset  ANTHROPIC_API_KEY\n",
		"set    ANTHROPIC_BASE_URL=http://127.0.0.1:8317\n",
		"       ANTHROPIC_AUTH_TOKEN=<file ~/.config/graft/keys/cliproxy, sha256:" + keys.Fingerprint(cliproxyKey) + ">\n",
		"exec   /opt/claude/bin/claude --model glm-5.3-flash --effort max --settings '{\"autoCompactEnabled\":true,",
		"' --resume\n",
		"settings\n{\n  \"autoCompactEnabled\": true,\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, cliproxyKey) {
		t.Error("--dry-run printed the key")
	}
}

func TestDryRunRouter(t *testing.T) {
	f := newFake(t, "OLLAMA_API_KEY=sk-ollama", "ANTHROPIC_API_KEY=sk-ant-leak")
	f.writeConfig(t)
	if code := run([]string{"--dry-run", "mix"}, f.sys); code != 0 {
		t.Fatalf("code %d, stderr %s", code, f.stderr.String())
	}
	out := f.stdout.String()
	for _, want := range []string{
		"profile mix: router, provider ollama, subscription\n",
		"unset  ANTHROPIC_API_KEY\n",
		"set    ANTHROPIC_BASE_URL=http://127.0.0.1:<port>\n",
		"       ANTHROPIC_CUSTOM_HEADERS=X-Graft-Session: <session secret>\n",
		"       NO_PROXY=127.0.0.1\n",
		"route  glm -> ollama glm-5.3-flash <env OLLAMA_API_KEY, sha256:" + keys.Fingerprint("sk-ollama") + ">\n",
		"exec   /opt/claude/bin/claude --model opus --effort max --settings '{",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "sk-ollama") {
		t.Error("--dry-run printed the key")
	}
}

// TestKeysAreReadOnce: a key command may ask for a password, so graft runs
// it once however many models the provider serves.
func TestKeysAreReadOnce(t *testing.T) {
	f := newFake(t)
	count := filepath.Join(t.TempDir(), "count")
	t.Setenv("COUNT", count)
	writeFile(t, filepath.Join(f.sys.home, ".config", "graft", "config.yaml"), `
providers:
  counted: { url: "http://127.0.0.1:8317", key: { command: 'echo run >> "$COUNT"; echo sk-counted' } }
models:
  a: { provider: counted, context: 200000 }
  b: { provider: counted, context: 200000 }
profiles:
  two: { model: a, picker: [b], subscription: true }
`, 0o600)
	if code := run([]string{"--dry-run", "two"}, f.sys); code != 0 {
		t.Fatalf("code %d, stderr %s", code, f.stderr.String())
	}
	if got := readFile(t, count); got != "run\n" {
		t.Errorf("the key command ran %d times, want once", strings.Count(got, "run"))
	}
}

// TestRouterProfileRuns runs graft for real with a profile behind the
// router: this test binary plays claude, which sends one request.
func TestRouterProfileRuns(t *testing.T) {
	var mu sync.Mutex
	var gotKey, gotBody, gotHost string
	answer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotKey, gotBody, gotHost = r.Header.Get("Authorization"), string(body), r.Host
		mu.Unlock()
		io.WriteString(w, `{"ok":true}`)
	})
	provider := httptest.NewServer(answer)
	defer provider.Close()
	proxy := httptest.NewServer(answer) // a forward proxy answers for the provider
	defer proxy.Close()
	state := t.TempDir()
	notDir := filepath.Join(state, "file")
	writeFile(t, notDir, "", 0o600)

	tests := []struct {
		name, url, state string
		environ          []string
		host             string // the provider's host as the request names it
		log              bool
	}{
		{"with a log", provider.URL, state, nil, strings.TrimPrefix(provider.URL, "http://"), true},
		{"without a log", provider.URL, notDir, nil, strings.TrimPrefix(provider.URL, "http://"), false},
		{"through a proxy", "http://provider.invalid", state, []string{"HTTP_PROXY=" + proxy.URL}, "provider.invalid", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderr, out := runGraft(t, tt.url, tt.state, "", tt.environ...)
			if answer := readFile(t, filepath.Join(out, "answer")); answer != `200 {"ok":true}` {
				t.Errorf("claude got %q", answer)
			}
			mu.Lock()
			if gotKey != "Bearer sk-local" || gotBody != `{"model":"glm-5.3-flash"}` || gotHost != tt.host {
				t.Errorf("the provider got %q and %q for %s", gotKey, gotBody, gotHost)
			}
			mu.Unlock()
			// Through a proxy, Claude Code would not reach the router.
			if got := readFile(t, filepath.Join(out, "no_proxy")); got != "127.0.0.1 127.0.0.1" {
				t.Errorf("claude got NO_PROXY and no_proxy %q, want 127.0.0.1 in both", got)
			}
			// A key command may ask for a password: once for both models.
			if runs := readFile(t, filepath.Join(out, "runs")); runs != "run\n" {
				t.Errorf("the key command ran %d times, want once", strings.Count(runs, "run"))
			}
			warned := strings.Contains(stderr, "graft: warning: cannot open the log")
			if warned == tt.log || strings.Contains(stderr, "saves model") {
				t.Errorf("stderr:\n%s", stderr)
			}
			if !tt.log {
				return
			}
			log := readFile(t, filepath.Join(state, "graft", "graft.log"))
			if !strings.Contains(log, " start profile=sub port=") || !strings.Contains(log, " end exit=5\n") ||
				strings.Contains(log, "sk-local") {
				t.Errorf("log:\n%s", log)
			}
		})
	}
}

// TestRouterSendsHTTPSThroughHTTPProxy: without HTTPS_PROXY, Claude Code
// sends https through HTTP_PROXY. So must the router, or a network that lets
// out only the proxy would serve direct profiles and fail the router's.
func TestRouterSendsHTTPSThroughHTTPProxy(t *testing.T) {
	tests := []struct {
		name, url string
		environ   []string // next to HTTP_PROXY
		want      []string // what the proxy gets
	}{
		{"HTTP_PROXY alone", "https://provider.invalid", nil, []string{"CONNECT provider.invalid:443"}},
		// The entry has the port, so that it holds only for https. 0.0.0.0
		// needs no DNS, and Go's proxy rules do not count it as loopback;
		// direct, the router finds no server there and stays on this machine.
		{"the provider in NO_PROXY", "https://0.0.0.0", []string{"NO_PROXY=0.0.0.0:443"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var got []string
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				got = append(got, r.Method+" "+r.Host)
				mu.Unlock()
				http.Error(w, "no tunnels here", http.StatusForbidden)
			}))
			defer proxy.Close()
			_, out := runGraft(t, tt.url, t.TempDir(), "", append([]string{"HTTP_PROXY=" + proxy.URL}, tt.environ...)...)
			mu.Lock()
			defer mu.Unlock()
			if !slices.Equal(got, tt.want) {
				t.Errorf("the proxy got %q, want %q", got, tt.want)
			}
			// Refused by the proxy or by this machine, the provider fails with the router's 502.
			if answer := readFile(t, filepath.Join(out, "answer")); !strings.HasPrefix(answer, "502 ") {
				t.Errorf("claude got %q", answer)
			}
		})
	}
}

// TestRouterWarnsOfASavedModel: Enter in /model saves the model for new
// sessions in Claude Code's user settings. Behind the router that is a graft
// model, which a plain claude would ask Anthropic for, so graft says so once
// claude has exited and the terminal is free.
func TestRouterWarnsOfASavedModel(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ok":true}`)
	}))
	defer provider.Close()
	t.Run("the settings of a plain claude", func(t *testing.T) {
		stderr, _ := runGraft(t, provider.URL, t.TempDir(), "", "FAKE_SAVE_MODEL=glm-air")
		want := "graft: warning: ~/.claude/settings.json saves model \"glm-air\" for new sessions, and only graft serves it: " +
			"a plain claude would ask Anthropic for it. Remove \"model\" from that file, or pick a Claude model in /model of a plain claude; " +
			"in /model, s picks a model for this session only\n"
		if !strings.HasSuffix(stderr, want) {
			t.Errorf("stderr:\n%s\nwant it to end with:\n%s", stderr, want)
		}
	})
	// A plain claude does not read the settings dir of graft's sessions.
	t.Run("graft's own settings dir", func(t *testing.T) {
		dir := t.TempDir()
		stderr, _ := runGraft(t, provider.URL, t.TempDir(), "defaults: { env: { CLAUDE_CONFIG_DIR: \""+dir+"\" } }\n", "FAKE_SAVE_MODEL=glm-air")
		if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
			t.Fatalf("the fake claude saved nothing in its settings dir: %v", err)
		}
		if strings.Contains(stderr, "saves model") {
			t.Errorf("stderr:\n%s", stderr)
		}
	})
}

// runGraft runs graft for real, as a child of this test, with the profile
// sub behind the router: models glm and glm-air at the provider url, on the
// subscription, with the log in state and defaults, if not "", as the
// config's defaults block. The provider's key comes from a
// command that notes each of its runs in the file runs. This test binary
// plays claude: see fakeClaude. runGraft returns graft's stderr and the
// directory with the files of the fake claude and of the key command.
func runGraft(t *testing.T, url, state, defaults string, environ ...string) (string, string) {
	t.Helper()
	home, bin, out := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.Symlink(os.Args[0], filepath.Join(bin, "claude")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".config", "graft", "config.yaml"), `
providers:
  local: { url: "`+url+`", key: { command: 'echo run >> "$FAKE_OUT/runs"; echo sk-local' } }
models:
  glm:     { provider: local, id: glm-5.3-flash, context: 200000 }
  glm-air: { provider: local, id: glm-5.3-air, context: 200000 }
profiles:
  sub: { model: glm, picker: [glm-air], subscription: true }
`+defaults, 0o600)
	cmd := exec.Command(os.Args[0], "sub")
	cmd.Dir = t.TempDir()
	cmd.Env = append([]string{
		"GRAFT_TEST_RUN_MAIN=1",
		"HOME=" + home,
		"PATH=" + bin + ":/usr/bin:/bin",
		"XDG_STATE_HOME=" + state,
		"FAKE_OUT=" + out,
	}, environ...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	var exitErr *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 5 {
		t.Fatalf("graft ended with %v, want claude's exit code 5; stderr:\n%s", err, stderr.String())
	}
	return stderr.String(), out
}

func TestDryRunNeedsClaude(t *testing.T) {
	f := newFake(t)
	f.writeConfig(t)
	f.sys.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if code := run([]string{"--dry-run", "glm-proxy"}, f.sys); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
	if !strings.Contains(f.stderr.String(), "claude not found in PATH") {
		t.Errorf("stderr %q", f.stderr.String())
	}
}

func TestCheck(t *testing.T) {
	f := newFake(t)
	f.writeConfig(t)
	if code := run([]string{"check"}, f.sys); code != 1 {
		t.Fatalf("code %d, want 1: the ollama key is missing", code)
	}
	want := "config ~/.config/graft/config.yaml: ok\n" +
		"provider cliproxy: key ok (file ~/.config/graft/keys/cliproxy, sha256:" + keys.Fingerprint(cliproxyKey) + ")\n" +
		"provider ollama: error: env OLLAMA_API_KEY is not set\n" +
		"claude: /opt/claude/bin/claude 2.1.296\n"
	if f.stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", f.stdout.String(), want)
	}
}

func TestCheckWarnings(t *testing.T) {
	f := newFake(t, "OLLAMA_API_KEY=sk-ollama")
	f.writeConfig(t)
	f.sys.claudeVersion = func(string) (string, error) { return "2.1.250", nil }
	writeFile(t, filepath.Join(f.sys.home, ".claude", "settings.json"), `{"apiKeyHelper": "get-key", "model": "glm-proxy"}`, 0o600)
	if code := run([]string{"check"}, f.sys); code != 0 {
		t.Fatalf("code %d, want 0: warnings do not fail check\n%s", code, f.stdout.String())
	}
	for _, want := range []string{
		"claude: warning: /opt/claude/bin/claude is version 2.1.250; graft needs 2.1.288 or later\n",
		"claude settings: warning: profiles with external models will not start here because of ~/.claude/settings.json: apiKeyHelper\n",
		// Enter in /model behind the router saves the name of a graft model.
		"claude settings: warning: ~/.claude/settings.json saves model \"glm-proxy\" for new sessions, and only graft serves it: " +
			"a plain claude would ask Anthropic for it. Remove \"model\" from that file, or pick a Claude model in /model of a plain claude; " +
			"in /model, s picks a model for this session only\n",
	} {
		if !strings.Contains(f.stdout.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, f.stdout.String())
		}
	}
}

// TestCheckWarnsOfThePlainClaudeSettings: check looks for a saved graft
// model in the settings a plain claude reads, not in the settings dir that
// the defaults give graft's sessions.
func TestCheckWarnsOfThePlainClaudeSettings(t *testing.T) {
	f := newFake(t, "OLLAMA_API_KEY=sk-ollama")
	f.writeConfig(t)
	graftDir := t.TempDir()
	writeFile(t, filepath.Join(f.sys.home, ".config", "graft", "config.yaml"), strings.Replace(testConfig,
		"defaults:\n  effort: max\n", "defaults:\n  effort: max\n  env: { CLAUDE_CONFIG_DIR: \""+graftDir+"\" }\n", 1), 0o600)
	writeFile(t, filepath.Join(graftDir, "settings.json"), `{"model": "glm"}`, 0o600)
	if code := run([]string{"check"}, f.sys); code != 0 || strings.Contains(f.stdout.String(), "saves model") {
		t.Fatalf("code %d, output:\n%s", code, f.stdout.String())
	}
	writeFile(t, filepath.Join(f.sys.home, ".claude", "settings.json"), `{"model": "glm-proxy"}`, 0o600)
	f.stdout.Reset()
	if code := run([]string{"check"}, f.sys); code != 0 || !strings.Contains(f.stdout.String(), `~/.claude/settings.json saves model "glm-proxy"`) {
		t.Errorf("code %d, output:\n%s", code, f.stdout.String())
	}
}

func TestCheckWithoutClaude(t *testing.T) {
	f := newFake(t, "OLLAMA_API_KEY=sk-ollama")
	f.writeConfig(t)
	f.sys.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if code := run([]string{"check"}, f.sys); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
	if !strings.Contains(f.stdout.String(), "claude: error: not found in PATH\n") {
		t.Errorf("stdout:\n%s", f.stdout.String())
	}
}

func TestOlderThan(t *testing.T) {
	tests := []struct {
		v    string
		want bool
	}{
		{"2.1.288", false}, {"2.1.296", false}, {"2.2.0", false}, {"3.0", false},
		{"2.1.287", true}, {"2.0.999", true}, {"1.9", true}, {"garbage", true},
	}
	for _, tt := range tests {
		if got := olderThan(tt.v, minClaude); got != tt.want {
			t.Errorf("olderThan(%q) = %v, want %v", tt.v, got, tt.want)
		}
	}
}

// TestExecReplacesProcess runs graft for real: graft must replace itself
// with claude, so claude's exit code becomes graft's.
func TestExecReplacesProcess(t *testing.T) {
	home, bin, out := t.TempDir(), t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(bin, "claude"),
		"#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FAKE_OUT/args\"\nenv > \"$FAKE_OUT/env\"\nexit 7\n", 0o755)
	dir := filepath.Join(home, ".config", "graft")
	writeFile(t, filepath.Join(dir, "config.yaml"), testConfig, 0o600)
	writeFile(t, filepath.Join(dir, "keys", "cliproxy"), cliproxyKey+"\n", 0o600)

	cmd := exec.Command(os.Args[0], "glm-proxy", "--resume")
	cmd.Dir = t.TempDir()
	cmd.Env = []string{
		"GRAFT_TEST_RUN_MAIN=1",
		"HOME=" + home,
		"PATH=" + bin + ":/usr/bin:/bin",
		"FAKE_OUT=" + out,
		"ANTHROPIC_API_KEY=sk-ant-leak",
	}
	var exitErr *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("graft ended with %v, want claude's exit code 7", err)
	}
	args := readFile(t, filepath.Join(out, "args"))
	if !strings.HasPrefix(args, "--model\nglm-5.3-flash\n--effort\nmax\n--settings\n{") || !strings.HasSuffix(args, "\n--resume\n") {
		t.Errorf("claude got the arguments:\n%s", args)
	}
	if strings.Contains(args, cliproxyKey) {
		t.Error("the key is on the command line")
	}
	env := readFile(t, filepath.Join(out, "env"))
	if !strings.Contains(env, "ANTHROPIC_AUTH_TOKEN="+cliproxyKey+"\n") {
		t.Error("claude did not get the key")
	}
	if strings.Contains(env, "ANTHROPIC_API_KEY=") {
		t.Error("ANTHROPIC_API_KEY from the shell reached claude")
	}
}
