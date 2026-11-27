//go:build !windows

package main

import (
	"os/exec"
	"runtime"
	"syscall"
)

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
