//go:build !windows

package launch

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// fakeSignalMode plays the modes of fakeClaude that need Unix signals:
// killed dies of SIGKILL, term reports the SIGTERM or SIGHUP it gets, and
// int reports a SIGINT and exits with 3 once the test lets it go.
func fakeSignalMode(mode string, write func(name, s string)) (code int, ok bool) {
	switch mode {
	case "killed":
		syscall.Kill(os.Getpid(), syscall.SIGKILL)
		time.Sleep(5 * time.Second)
		return 2, true
	case "term":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM, syscall.SIGHUP)
		write("ready", "")
		select {
		case sig := <-signals:
			write("got", sig.String())
		case <-time.After(5 * time.Second):
		}
		return 0, true
	case "int":
		if signal.Ignored(syscall.SIGINT) {
			write("got", "SIGINT ignored from the start")
		}
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT)
		write("ready", "")
		out := os.Getenv("FAKE_OUT")
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			select {
			case <-signals:
				write("got", "SIGINT")
			default:
			}
			if _, err := os.Stat(filepath.Join(out, "go")); err == nil {
				return 3, true
			}
		}
		return 3, true
	}
	return 0, false
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
