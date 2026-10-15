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

// established is the state of a connected TCP socket in /proc/net/tcp. The
// other states are those of a socket that is connecting, listening or closing.
const established = 0x01

// noUser stands in for a uid in the refusal of a connection that had closed,
// which has no other user to name.
const noUser = -1

// userListener drops the connections of processes that belong to other users
// of the machine. It trusts what the kernel says of a client only while the
// connection is open: a client that wrote its request and closed is listed as
// a time-wait socket of uid 0, or, after a reset, not at all, while its request
// waits in the server's buffer. So a connection passes only if the server's
// socket is ESTABLISHED and the client's socket, when the tables list it, is
// ESTABLISHED and belongs to root or to the user of graft ui. A client that the
// tables do not list passes, as it may be relayed from another network
// namespace.
type userListener struct {
	net.Listener
	uid   int                    // the user of graft ui
	table func() ([]byte, error) // the text of /proc/net/tcp followed by that of /proc/net/tcp6
	log   io.Writer

	mu      sync.Mutex
	refused map[int]bool // the users already reported, and noUser once a closed connection has been
}

func (l *userListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		uid, ok := l.peer(c)
		if ok {
			return c, nil
		}
		c.Close()
		l.report(uid)
	}
}

// report tells the terminal of a refused connection: once for each other user,
// and once for the connections that had closed.
func (l *userListener) report(uid int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.refused[uid] {
		return
	}
	l.refused[uid] = true
	if uid == noUser {
		fmt.Fprintln(l.log, "graft ui: refused a connection that closed before its user could be checked")
		return
	}
	fmt.Fprintf(l.log, "graft ui: refused a connection from uid %d\n", uid)
}

// peer checks the connection c. It returns true if c may be served. If not, it
// also returns why: the uid of the other user that c belongs to, or noUser if c
// had closed or could not be checked.
func (l *userListener) peer(c net.Conn) (int, bool) {
	server, ok1 := c.LocalAddr().(*net.TCPAddr)
	client, ok2 := c.RemoteAddr().(*net.TCPAddr)
	if !ok1 || !ok2 {
		return noUser, false
	}
	table, err := l.table()
	if err != nil {
		return noUser, false
	}
	// The client's socket has the client as its local address and the server
	// as its remote one. An AF_INET6 socket connected to an IPv4 address is
	// listed in tcp6 only, under the IPv4-mapped form of both addresses.
	row, found := findSocket(table, procAddr(client), procAddr(server))
	if !found {
		row, found = findSocket(table, procAddr6(client), procAddr6(server))
	}
	if found {
		if row.state != established {
			return noUser, false
		}
		if row.uid != l.uid && row.uid != 0 {
			return row.uid, false
		}
	}
	// The server's own socket is checked last, in a second read. One read is
	// no snapshot: it takes milliseconds, and a client that resets while the
	// tables are read can be missing from the text although the server's row,
	// read earlier, still says ESTABLISHED. The second read shows the reset.
	// The server's socket is an IPv4 one, as the listener is, so its row is
	// in tcp.
	table, err = l.table()
	if err != nil {
		return noUser, false
	}
	srv, listed := findSocket(table, procAddr(server), procAddr(client))
	if !listed || srv.state != established {
		return noUser, false
	}
	return 0, true
}

// socketRow is what a line of /proc/net/tcp or /proc/net/tcp6 says of a
// socket.
type socketRow struct {
	state int // the TCP state: established is 1
	uid   int // the owner; 0 for a time-wait socket, whoever owned it
}

// findSocket finds in table, the text of /proc/net/tcp and /proc/net/tcp6, the
// socket that is at the address local and connected to remote, both written as
// procAddr or procAddr6 write them.
func findSocket(table []byte, local, remote string) (socketRow, bool) {
	if local == "" || remote == "" {
		return socketRow{}, false
	}
	for _, line := range strings.Split(string(table), "\n") {
		f := strings.Fields(line)
		if len(f) < 8 || f[1] != local || f[2] != remote {
			continue
		}
		state, err1 := strconv.ParseUint(f[3], 16, 8)
		uid, err2 := strconv.Atoi(f[7])
		if err1 != nil || err2 != nil {
			continue
		}
		return socketRow{int(state), uid}, true
	}
	return socketRow{}, false
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

// procAddr6 writes an address as /proc/net/tcp6 does: the 128 bits of the
// address as four words, each in the machine's byte order, then the port, all
// in hex. An IPv4 address is written as its IPv4-mapped address, which is how
// the kernel lists an AF_INET6 socket that is connected over IPv4.
func procAddr6(a *net.TCPAddr) string {
	ip := a.IP.To16()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%08X%08X%08X%08X:%04X",
		binary.NativeEndian.Uint32(ip[0:4]), binary.NativeEndian.Uint32(ip[4:8]),
		binary.NativeEndian.Uint32(ip[8:12]), binary.NativeEndian.Uint32(ip[12:16]), a.Port)
}
