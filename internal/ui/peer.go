package ui

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

// userListener drops the connections of processes that belong to other
// users of the machine. Root and the user of graft ui pass, and so does a
// connection whose socket the table lacks, such as one relayed from
// another network namespace.
type userListener struct {
	net.Listener
	uid   int                    // the user of graft ui
	table func() ([]byte, error) // the text of /proc/net/tcp
	log   io.Writer

	mu      sync.Mutex
	refused map[int]bool // the users already reported
}

func (l *userListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		uid, found := l.peer(c)
		if !found || uid == l.uid || uid == 0 {
			return c, nil
		}
		c.Close()
		l.mu.Lock()
		if !l.refused[uid] {
			l.refused[uid] = true
			fmt.Fprintf(l.log, "graft ui: refused a connection from uid %d\n", uid)
		}
		l.mu.Unlock()
	}
}

// peer returns the user of the process at the other end of c.
func (l *userListener) peer(c net.Conn) (int, bool) {
	server, ok1 := c.LocalAddr().(*net.TCPAddr)
	client, ok2 := c.RemoteAddr().(*net.TCPAddr)
	if !ok1 || !ok2 {
		return 0, false
	}
	table, err := l.table()
	if err != nil {
		return 0, false
	}
	// The client's socket: its local address is the client, its remote
	// address the server.
	return socketUID(table, client, server)
}

// socketUID finds in table, the text of /proc/net/tcp, the socket with the
// addresses local and remote, and returns the user that owns it.
func socketUID(table []byte, local, remote *net.TCPAddr) (int, bool) {
	l, r := procAddr(local), procAddr(remote)
	if l == "" || r == "" {
		return 0, false
	}
	for _, line := range strings.Split(string(table), "\n") {
		f := strings.Fields(line)
		if len(f) < 8 || f[1] != l || f[2] != r {
			continue
		}
		uid, err := strconv.Atoi(f[7])
		return uid, err == nil
	}
	return 0, false
}

// procAddr writes an IPv4 address as /proc/net/tcp does: the 32 bits of the
// address in the machine's byte order, then the port, both in hex.
func procAddr(a *net.TCPAddr) string {
	ip := a.IP.To4()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%08X:%04X", binary.NativeEndian.Uint32(ip), a.Port)
}
