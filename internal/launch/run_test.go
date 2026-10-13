package launch

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/plan"
)

func TestMain(m *testing.M) {
	// Run starts this test binary as claude: see fakeClaude.
	if filepath.Base(os.Args[0]) == "claude" {
		os.Exit(fakeClaude())
	}
	os.Exit(m.Run())
}

// fakeClaude plays claude for the tests of Run, as FAKE_CLAUDE says, and
// returns its exit code. It leaves what it saw in files in FAKE_OUT.
func fakeClaude() int {
	out := os.Getenv("FAKE_OUT")
	write := func(name, s string) { os.WriteFile(filepath.Join(out, name), []byte(s), 0o600) }
	switch os.Getenv("FAKE_CLAUDE") {
	case "exit 7":
		return 7
	case "killed":
		syscall.Kill(os.Getpid(), syscall.SIGKILL)
		time.Sleep(5 * time.Second)
	case "term":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM, syscall.SIGHUP)
		write("ready", "")
		select {
		case sig := <-signals:
			write("got", sig.String())
		case <-time.After(5 * time.Second):
		}
		return 0
	case "int":
		if signal.Ignored(syscall.SIGINT) {
			write("got", "SIGINT ignored from the start")
		}
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT)
		write("ready", "")
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			select {
			case <-signals:
				write("got", "SIGINT")
			default:
			}
			if _, err := os.Stat(filepath.Join(out, "go")); err == nil {
				return 3
			}
		}
		return 3
	case "request":
		base := os.Getenv("ANTHROPIC_BASE_URL")
		name, value, _ := strings.Cut(os.Getenv("ANTHROPIC_CUSTOM_HEADERS"), ": ")
		write("base", base)
		write("secret", value)
		req, _ := http.NewRequest(http.MethodPost, base+"/v1/messages", strings.NewReader(`{"model":"glm"}`))
		req.Header.Set(name, value)
		req.Header.Set("Authorization", "Bearer "+os.Getenv("ANTHROPIC_AUTH_TOKEN"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			write("answer", err.Error())
			return 1
		}
		body, _ := io.ReadAll(resp.Body)
		write("answer", fmt.Sprintf("%d %s", resp.StatusCode, body))
		return 0
	}
	return 2
}

// routerPlan is the plan of a router profile whose model glm goes to the
// provider ollama at url.
func routerPlan(url string) *plan.Plan {
	return &plan.Plan{
		Profile: "multi",
		Mode:    config.Router,
		Routes:  []plan.Route{{Model: "glm", Provider: "ollama", URL: url, ID: "glm-5.3-flash"}},
		Args:    []string{"--model", "glm"},
		Set: []plan.EnvVar{
			{Name: "ANTHROPIC_BASE_URL", Session: plan.SessionURL},
			{Name: "ANTHROPIC_CUSTOM_HEADERS", Session: plan.SessionHeader},
			{Name: "ANTHROPIC_AUTH_TOKEN", Value: "graft"},
		},
	}
}

func ollamaKey(provider string) (string, error) {
	if provider != "ollama" {
		return "", errors.New("unexpected provider " + provider)
	}
	return "sk-ollama", nil
}

// run calls Run with this test binary as claude, which plays the part
// FAKE_CLAUDE names, and returns the exit code and the files it left.
func run(t *testing.T, p *plan.Plan, part string, log io.Writer, during func(out string)) (int, string) {
	t.Helper()
	out := t.TempDir()
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := Run(p, []string{"FAKE_CLAUDE=" + part, "FAKE_OUT=" + out}, ollamaKey, RunConfig{
			LookPath: func(string) (string, error) { return os.Executable() },
			Log:      log,
		})
		done <- result{code, err}
	}()
	if during != nil {
		during(out)
	}
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r.code, out
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not return")
	}
	return 0, ""
}

// waitFor waits until the fake claude has left the file name in out.
func waitFor(t *testing.T, out, name string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			return
		}
	}
	t.Fatalf("no %s from the fake claude", name)
}

func readOut(t *testing.T, out, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, name))
	if err != nil {
		return ""
	}
	return string(data)
}

func TestRunServesClaude(t *testing.T) {
	var mu sync.Mutex
	var gotAuth, gotBody string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotAuth, gotBody = r.Header.Get("Authorization"), string(body)
		mu.Unlock()
		io.WriteString(w, `{"ok":true}`)
	}))
	defer provider.Close()
	logFile, err := os.Create(filepath.Join(t.TempDir(), "graft.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	code, out := run(t, routerPlan(provider.URL), "request", logFile, nil)
	if code != 0 {
		t.Fatalf("exit code %d, answer %q", code, readOut(t, out, "answer"))
	}
	if answer := readOut(t, out, "answer"); answer != `200 {"ok":true}` {
		t.Errorf("claude got %q", answer)
	}
	mu.Lock()
	if gotAuth != "Bearer sk-ollama" || gotBody != `{"model":"glm-5.3-flash"}` {
		t.Errorf("the provider got %q and %q", gotAuth, gotBody)
	}
	mu.Unlock()
	base := readOut(t, out, "base")
	if !regexp.MustCompile(`^http://127\.0\.0\.1:\d+$`).MatchString(base) {
		t.Errorf("ANTHROPIC_BASE_URL = %q", base)
	}
	if conn, err := net.Dial("tcp", strings.TrimPrefix(base, "http://")); err == nil {
		conn.Close()
		t.Error("the router still listens after claude has exited")
	}
	secret := readOut(t, out, "secret")
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(secret) {
		t.Errorf("the session secret %q is not 32 random bytes in hex", secret)
	}

	data, err := os.ReadFile(logFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	for _, want := range []*regexp.Regexp{
		regexp.MustCompile(`session=([0-9a-f]{6}) start profile=multi port=\d+ models=glm>ollama\n`),
		regexp.MustCompile(`session=[0-9a-f]{6} POST /v1/messages model=glm route=ollama status=200 `),
		regexp.MustCompile(`session=[0-9a-f]{6} end exit=0\n$`),
	} {
		if !want.MatchString(log) {
			t.Errorf("the log lacks %s:\n%s", want, log)
		}
	}
	if session := regexp.MustCompile(`session=(\w+)`).FindStringSubmatch(log); session == nil || strings.Contains(secret, session[1]) {
		t.Errorf("the session ID must not come from the secret:\n%s", log)
	}
	if strings.Contains(log, secret) || strings.Contains(log, "sk-ollama") {
		t.Errorf("the log shows the secret or the key:\n%s", log)
	}
}

func TestRunExitCode(t *testing.T) {
	if code, _ := run(t, routerPlan("http://127.0.0.1:9"), "exit 7", nil, nil); code != 7 {
		t.Errorf("exit code %d, want claude's 7", code)
	}
}

func TestRunClaudeKilledBySignal(t *testing.T) {
	if code, _ := run(t, routerPlan("http://127.0.0.1:9"), "killed", nil, nil); code != 128+int(syscall.SIGKILL) {
		t.Errorf("exit code %d, want 128+9", code)
	}
}

func TestRunPassesSignalsOn(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			code, out := run(t, routerPlan("http://127.0.0.1:9"), "term", nil, func(out string) {
				waitFor(t, out, "ready")
				syscall.Kill(os.Getpid(), sig) // to graft, which runs in this process
			})
			if got := readOut(t, out, "got"); code != 0 || got != sig.String() {
				t.Errorf("exit code %d, claude got %q; want %v passed on", code, got, sig)
			}
		})
	}
}

// TestRunLeavesSIGINT: the terminal sends Ctrl+C to claude and graft both,
// so graft must neither pass it on nor die of it.
func TestRunLeavesSIGINT(t *testing.T) {
	code, out := run(t, routerPlan("http://127.0.0.1:9"), "int", nil, func(out string) {
		waitFor(t, out, "ready")
		syscall.Kill(os.Getpid(), syscall.SIGINT)
		syscall.Kill(os.Getpid(), syscall.SIGQUIT)
		time.Sleep(200 * time.Millisecond)
		os.WriteFile(filepath.Join(out, "go"), nil, 0o600)
	})
	if got := readOut(t, out, "got"); code != 3 || got != "" {
		t.Errorf("exit code %d, claude got %q; want claude's 3 and no signal from graft", code, got)
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name     string
		lookPath func(string) (string, error)
		key      KeyFunc
		want     string
	}{
		{"no claude", func(string) (string, error) { return "", errors.New("not found") }, ollamaKey, "claude not found in PATH"},
		{"no key", func(string) (string, error) { return os.Executable() },
			func(string) (string, error) { return "", errors.New("provider ollama: env OLLAMA_API_KEY is not set") },
			"provider ollama: env OLLAMA_API_KEY is not set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := t.TempDir()
			_, err := Run(routerPlan("http://127.0.0.1:9"), []string{"FAKE_CLAUDE=exit 7", "FAKE_OUT=" + out}, tt.key,
				RunConfig{LookPath: tt.lookPath})
			if err == nil || err.Error() != tt.want {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}
}
