package launch

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/zinin/graft/internal/plan"
	"github.com/zinin/graft/internal/router"
)

// RunConfig is what Run takes from the outside world.
type RunConfig struct {
	LookPath func(string) (string, error) // exec.LookPath outside tests
	Stdin    io.Reader                    // claude's terminal
	Stdout   io.Writer
	Stderr   io.Writer
	Log      io.Writer // graft's log file; nil for none
}

// Run starts the router and claude as its child, waits for claude and stops
// the router. It returns claude's exit code, or 128+N when signal N killed
// claude. An error means that claude did not start.
func Run(p *plan.Plan, environ []string, key KeyFunc, rc RunConfig) (int, error) {
	path, err := rc.LookPath("claude")
	if err != nil {
		return 0, errors.New("claude not found in PATH")
	}
	routes := map[string]router.Route{}
	var models []string
	for _, r := range p.Routes {
		k, err := key(r.Provider)
		if err != nil {
			return 0, err
		}
		u, err := url.Parse(r.URL)
		if err != nil {
			return 0, fmt.Errorf("provider %s: %w", r.Provider, err)
		}
		routes[r.Model] = router.Route{Provider: r.Provider, URL: u, Key: k, ID: r.ID}
		models = append(models, r.Model+">"+r.Provider)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("cannot open the router's socket: %w", err)
	}
	// The server closes it too, but only once Serve has started.
	defer ln.Close()
	s := &Session{Port: ln.Addr().(*net.TCPAddr).Port, Secret: randomHex(32)}
	env, err := Environ(environ, p, key, s)
	if err != nil {
		return 0, err
	}
	// The session ID only marks the lines of the session in the log: it has
	// nothing to do with the secret.
	log := router.NewLog(rc.Log, randomHex(3))
	srv := router.Server(router.Config{
		Profile:      p.Profile,
		Subscription: p.Subscription,
		Routes:       routes,
		Secret:       s.Secret,
		Log:          log,
	})
	log.Printf("start profile=%s port=%d models=%s", p.Profile, s.Port, strings.Join(models, ","))
	go srv.Serve(ln)

	cmd := exec.Command(path, p.Args...)
	cmd.Args[0] = "claude"
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = rc.Stdin, rc.Stdout, rc.Stderr
	// Signals are caught before claude starts, so that none finds graft
	// unprepared. SIGINT and SIGQUIT come from the terminal to claude and
	// graft both: graft drops them. It catches rather than ignores them,
	// since claude would inherit an ignored signal.
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		srv.Close()
		return 0, fmt.Errorf("start %s: %w", path, err)
	}
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()
	for {
		select {
		case sig := <-signals:
			if sig == syscall.SIGTERM || sig == syscall.SIGHUP {
				cmd.Process.Signal(sig)
			}
		case <-done:
			// Closing the server cancels the requests still under way.
			srv.Close()
			code := exitCode(cmd.ProcessState)
			log.Printf("end exit=%d", code)
			return code, nil
		}
	}
}

// exitCode returns claude's exit code, or 128+N when signal N killed it, as
// a shell reports it.
func exitCode(ps *os.ProcessState) int {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}

// randomHex returns n random bytes in hex.
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
