package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/keys"
)

func TestMain(m *testing.M) {
	// TestExecReplacesProcess runs this test binary as graft itself.
	if os.Getenv("GRAFT_TEST_RUN_MAIN") == "1" {
		main()
	}
	os.Exit(m.Run())
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
		{[]string{"mix"}, `graft: profile "mix" needs the router, which is not implemented yet`},
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
		"claude: /opt/claude/bin/claude 2.1.296\n" +
		"profile mix: needs the router, which is not implemented yet\n"
	if f.stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", f.stdout.String(), want)
	}
}

func TestCheckWarnings(t *testing.T) {
	f := newFake(t, "OLLAMA_API_KEY=sk-ollama")
	f.writeConfig(t)
	f.sys.claudeVersion = func(string) (string, error) { return "2.1.250", nil }
	writeFile(t, filepath.Join(f.sys.home, ".claude", "settings.json"), `{"apiKeyHelper": "get-key"}`, 0o600)
	if code := run([]string{"check"}, f.sys); code != 0 {
		t.Fatalf("code %d, want 0: warnings do not fail check\n%s", code, f.stdout.String())
	}
	for _, want := range []string{
		"claude: warning: /opt/claude/bin/claude is version 2.1.250; graft needs 2.1.288 or later\n",
		"claude settings: warning: profiles with external models will not start here because of ~/.claude/settings.json: apiKeyHelper\n",
	} {
		if !strings.Contains(f.stdout.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, f.stdout.String())
		}
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

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"/bin/claude", "--model", "a b", "it's", "", `{"x":1}`})
	want := `/bin/claude --model 'a b' 'it'\''s' '' '{"x":1}'`
	if got != want {
		t.Errorf("shellJoin = %s, want %s", got, want)
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
