package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/ui"
)

// runUI serves the editor of the config at path until SIGINT or SIGTERM.
func runUI(path string, sys system) int {
	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	out := &lockedWriter{w: sys.stdout} // the server writes to it too
	srv, err := ui.New(ui.Options{Path: abs, Log: out, Environ: sys.environ, Stdin: sys.stdin, Stderr: sys.stderr})
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	signals, stop := sys.notify()
	defer stop()
	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()

	fmt.Fprintf(out, "graft ui: editing %s\n", config.Tildify(abs))
	fmt.Fprintf(out, "graft ui: open %s\n", srv.URL())
	if overSSH(sys) {
		fmt.Fprintf(out, "graft ui: from another machine: ssh -L %d:127.0.0.1:%d <host>, then open the address above\n",
			srv.Port(), srv.Port())
	}
	fmt.Fprintln(out, "graft ui: press Ctrl+C to stop")
	if canBrowse(sys) {
		sys.openBrowser(srv.URL())
	}

	select {
	case <-signals:
	case err := <-served:
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	return 0
}

func overSSH(sys system) bool {
	v, _ := sys.lookupEnv("SSH_CONNECTION")
	return v != ""
}

// canBrowse reports whether graft ui opens a browser itself: on macOS, and
// on Linux in a graphical session, but not over SSH.
func canBrowse(sys system) bool {
	if overSSH(sys) {
		return false
	}
	switch sys.goos {
	case "darwin":
		return true
	case "linux":
		x, _ := sys.lookupEnv("DISPLAY")
		wayland, _ := sys.lookupEnv("WAYLAND_DISPLAY")
		return x != "" || wayland != ""
	}
	return false
}

// openBrowser opens url in the default browser. It does not wait, and the
// browser does not write to the terminal; on failure the printed address
// is enough. The opener gets a session of its own: the Ctrl+C that stops
// graft ui goes to the whole foreground process group of the terminal, and
// would kill a browser that the opener has to start.
func openBrowser(url string) {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, url)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

func notifyStop() (<-chan os.Signal, func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	return c, func() { signal.Stop(c) }
}

// lockedWriter lets the server and runUI write lines to one terminal.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
