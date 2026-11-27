//go:build !linux

package ui

import (
	"io"
	"net"
)

// userGuard returns ln: macOS and Windows do not show the arguments of a
// process to other users, so they cannot read the token from the command
// line of open, or of rundll32 and the browser.
func userGuard(ln net.Listener, log io.Writer) net.Listener {
	return ln
}
