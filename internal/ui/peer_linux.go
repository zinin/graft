package ui

import (
	"fmt"
	"io"
	"net"
	"os"
)

// userGuard drops the connections of other users' processes. On Linux
// another user can read the URL with the token from the arguments of
// xdg-open in /proc, so the token alone is not enough.
func userGuard(ln net.Listener, log io.Writer) net.Listener {
	if _, err := procTables(); err != nil {
		fmt.Fprintf(log, "graft ui: warning: cannot read /proc/net/tcp, so graft ui cannot refuse other users of this machine who learn its address: %v\n", err)
		return ln
	}
	return &userListener{Listener: ln, uid: os.Getuid(), table: procTables, log: log, refused: map[int]bool{}}
}

// procTables returns the text of /proc/net/tcp followed by that of
// /proc/net/tcp6, which lists the IPv6 sockets in the same columns. IPv6 may be
// disabled, and then there is no tcp6: that counts as an empty table.
func procTables() ([]byte, error) {
	tcp, err := os.ReadFile("/proc/net/tcp")
	if err != nil {
		return nil, err
	}
	tcp6, err := os.ReadFile("/proc/net/tcp6")
	if err != nil {
		tcp6 = nil
	}
	return append(tcp, tcp6...), nil
}
