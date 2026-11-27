package main

import "os/exec"

// openBrowser opens url in the default browser through the URL handler of
// the shell, without cmd.exe, whose rules would take parts of an address for
// its own. It does not wait; on failure the printed address is enough.
// rundll32 and the browser are programs without a console, so the Ctrl+C
// that stops graft ui does not reach them.
func openBrowser(url string) {
	cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}
