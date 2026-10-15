package ui

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

const tableHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

// The TCP states, as /proc/net/tcp writes them, that the tests give to
// sockets besides established.
const (
	finWait1  = 0x04
	finWait2  = 0x05
	timeWait  = 0x06
	closeWait = 0x08
	closing   = 0x0B
)

const (
	ownUID   = 1000 // the user of graft ui
	otherUID = 4242 // another user of the machine

	otherLog  = "graft ui: refused a connection from uid 4242\n"
	closedLog = "graft ui: refused a connection that closed before its user could be checked\n"
)

// tableRow is a line of /proc/net/tcp for the socket at the address local that
// is connected to remote, in the TCP state st and owned by uid.
func tableRow(local, remote *net.TCPAddr, st, uid int) string {
	return rowText(procAddr(local), procAddr(remote), st, uid)
}

// tableRow6 is the line of /proc/net/tcp6 for an AF_INET6 socket that is
// connected to an IPv4 address: the kernel writes both addresses in their
// IPv4-mapped form.
func tableRow6(local, remote *net.TCPAddr, st, uid int) string {
	return rowText(procAddr6(local), procAddr6(remote), st, uid)
}

func rowText(local, remote string, st, uid int) string {
	return fmt.Sprintf("   0: %s %s %02X 00000000:00000000 00:00000000 00000000 %5d        0 1 1 0000000000000000 20 4 30 10 -1\n",
		local, remote, st, uid)
}

var (
	serverAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8000}
	clientAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5000}
)

// serverRow and clientRow are the lines for the two sockets of the connection
// between serverAddr and clientAddr. The server's socket belongs to the user
// of graft ui.
func serverRow(st int) string      { return tableRow(serverAddr, clientAddr, st, ownUID) }
func clientRow(st, uid int) string { return tableRow(clientAddr, serverAddr, st, uid) }

// clientRow6 is the client's line when its socket is an AF_INET6 one.
func clientRow6(st, uid int) string { return tableRow6(clientAddr, serverAddr, st, uid) }

func TestFindSocket(t *testing.T) {
	v6 := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5001}
	unlisted := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5002}
	table := []byte(tableHeader +
		tableRow(serverAddr, clientAddr, established, 1000) +
		tableRow(clientAddr, serverAddr, closing, 0) +
		tableRow6(v6, serverAddr, finWait2, 1001) +
		"   3: short line\n")
	tests := []struct {
		name          string
		local, remote string
		want          socketRow
		found         bool
	}{
		{"the server's socket", procAddr(serverAddr), procAddr(clientAddr), socketRow{established, 1000}, true},
		{"a state that the kernel writes with a letter", procAddr(clientAddr), procAddr(serverAddr), socketRow{closing, 0}, true},
		{"an IPv6 socket, by its mapped addresses", procAddr6(v6), procAddr6(serverAddr), socketRow{finWait2, 1001}, true},
		{"another port", procAddr(unlisted), procAddr(serverAddr), socketRow{}, false},
		{"the peer must match too", procAddr(clientAddr), procAddr(v6), socketRow{}, false},
		{"an IPv6 socket is not in the IPv4 form", procAddr(v6), procAddr(serverAddr), socketRow{}, false},
		{"an IPv4 socket is not in the mapped form", procAddr6(clientAddr), procAddr6(serverAddr), socketRow{}, false},
		{"an address that is not written", "", "", socketRow{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, found := findSocket(table, tt.local, tt.remote); got != tt.want || found != tt.found {
				t.Errorf("findSocket = %+v, %v; want %+v, %v", got, found, tt.want, tt.found)
			}
		})
	}
}

func TestProcAddrIsLittleEndianOnThisMachine(t *testing.T) {
	// graft is built for amd64 and arm64, both little-endian.
	if got := procAddr(clientAddr); got != "0100007F:1388" {
		t.Errorf("procAddr = %q", got)
	}
}

// TestProcAddr6 pins the text that the kernel writes into /proc/net/tcp6 for
// an AF_INET6 socket connected to ::ffff:127.0.0.1: four words in the
// machine's byte order, with the IPv4 address in the last one. It is the text
// of a real socket on Linux 7.0, and peer_linux_test.go finds a real socket by
// it.
func TestProcAddr6(t *testing.T) {
	tests := []struct {
		addr *net.TCPAddr
		want string
	}{
		{clientAddr, "0000000000000000FFFF00000100007F:1388"},
		{&net.TCPAddr{IP: net.IPv6loopback, Port: 5000}, "00000000000000000000000001000000:1388"},
		{&net.TCPAddr{Port: 5000}, ""},
	}
	for _, tt := range tests {
		if got := procAddr6(tt.addr); got != tt.want {
			t.Errorf("procAddr6(%v) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

type fakeConn struct {
	net.Conn
	client *net.TCPAddr // the other end; clientAddr when nil
	closed bool
}

func (c *fakeConn) LocalAddr() net.Addr { return serverAddr }
func (c *fakeConn) RemoteAddr() net.Addr {
	if c.client != nil {
		return c.client
	}
	return clientAddr
}
func (c *fakeConn) Close() error { c.closed = true; return nil }

type fakeListener struct {
	net.Listener
	conns []*fakeConn
}

func (l *fakeListener) Accept() (net.Conn, error) {
	if len(l.conns) == 0 {
		return nil, errors.New("no more connections")
	}
	c := l.conns[0]
	l.conns = l.conns[1:]
	return c, nil
}

// guardOf returns a userListener for the user ownUID that offers conns and
// reads the tables from table.
func guardOf(conns []*fakeConn, table func() ([]byte, error), log *lockedBuffer) *userListener {
	return &userListener{
		Listener: &fakeListener{conns: conns},
		uid:      ownUID,
		table:    table,
		log:      log,
		refused:  map[int]bool{},
	}
}

// accept offers one connection, from clientAddr to serverAddr, to a
// userListener whose tables have the lines rows. It returns whether the
// connection passed, and what the terminal shows.
func accept(t *testing.T, rows string) (passed bool, log string) {
	t.Helper()
	return acceptFrom(t, func() ([]byte, error) { return []byte(tableHeader + rows), nil })
}

// acceptFrom is accept for a table that changes from one read to the next.
func acceptFrom(t *testing.T, table func() ([]byte, error)) (passed bool, log string) {
	t.Helper()
	c, out := &fakeConn{}, &lockedBuffer{}
	got, err := guardOf([]*fakeConn{c}, table, out).Accept()
	if err == nil && got != net.Conn(c) {
		t.Fatalf("Accept = %v, want the connection offered", got)
	}
	passed = err == nil
	if passed == c.closed {
		t.Errorf("the connection passed: %v, and was closed: %v", passed, c.closed)
	}
	return passed, out.String()
}

func TestUserListenerRules(t *testing.T) {
	tests := []struct {
		name   string
		rows   string // the lines of /proc/net/tcp, then those of /proc/net/tcp6
		passes bool
		log    string
	}{
		{"the user's own process", serverRow(established) + clientRow(established, ownUID), true, ""},
		{"root", serverRow(established) + clientRow(established, 0), true, ""},
		{"another user", serverRow(established) + clientRow(established, otherUID), false, otherLog},
		{"another user, the client's line first", clientRow(established, otherUID) + serverRow(established), false, otherLog},

		// The client wrote its request and closed before Accept. Once the
		// server's kernel has acknowledged the FIN, the kernel writes the
		// closed client as a time-wait socket of uid 0.
		{"client in FIN_WAIT2 as uid 0, server in CLOSE_WAIT", serverRow(closeWait) + clientRow(finWait2, 0), false, closedLog},
		{"client in FIN_WAIT2 as uid 0", serverRow(established) + clientRow(finWait2, 0), false, closedLog},
		{"client in TIME_WAIT as uid 0", serverRow(established) + clientRow(timeWait, 0), false, closedLog},
		{"client in FIN_WAIT1 as its own user", serverRow(established) + clientRow(finWait1, ownUID), false, closedLog},
		{"client in FIN_WAIT1 as another user", serverRow(established) + clientRow(finWait1, otherUID), false, closedLog},

		// A client that reset its socket has no row, and neither has the
		// server's socket once it has seen the reset.
		{"client reset, server row gone", "", false, closedLog},
		{"client reset, server in CLOSE_WAIT", serverRow(closeWait), false, closedLog},

		{"server in CLOSE_WAIT", serverRow(closeWait) + clientRow(established, ownUID), false, closedLog},
		{"server row missing", clientRow(established, ownUID), false, closedLog},

		// An AF_INET6 client that connects to ::ffff:127.0.0.1 is in tcp6
		// only, in the IPv4-mapped form.
		{"IPv6 client of the user's own process", serverRow(established) + clientRow6(established, ownUID), true, ""},
		{"IPv6 client of root", serverRow(established) + clientRow6(established, 0), true, ""},
		{"IPv6 client of another user", serverRow(established) + clientRow6(established, otherUID), false, otherLog},
		{"IPv6 client in FIN_WAIT2 as uid 0", serverRow(established) + clientRow6(finWait2, 0), false, closedLog},
		{"IPv6 client, server in CLOSE_WAIT", serverRow(closeWait) + clientRow6(established, ownUID), false, closedLog},

		// A client in neither table is a relay from another network
		// namespace.
		{"client in neither table", serverRow(established), true, ""},
		{"another connection of another user", serverRow(established) +
			tableRow(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5001}, serverAddr, established, otherUID), true, ""},
		{"the same port at another address", serverRow(established) +
			tableRow(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 2), Port: 5000}, serverAddr, established, otherUID), true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			passed, log := accept(t, tt.rows)
			if passed != tt.passes || log != tt.log {
				t.Errorf("passed %v, log %q; want %v, %q", passed, log, tt.passes, tt.log)
			}
		})
	}
}

// TestUserListenerRefusesAClientThatIsNotEstablished: the kernel writes a
// closing socket with another uid than its owner's, so only an ESTABLISHED row
// can be trusted, whichever user it names.
func TestUserListenerRefusesAClientThatIsNotEstablished(t *testing.T) {
	for st := 0x01; st <= 0x0C; st++ {
		if st == established {
			continue
		}
		for _, uid := range []int{0, ownUID, otherUID} {
			for _, client := range []struct{ family, row string }{
				{"IPv4", clientRow(st, uid)},
				{"IPv6", clientRow6(st, uid)},
			} {
				if passed, log := accept(t, serverRow(established)+client.row); passed || log != closedLog {
					t.Errorf("%s client in state %02X as uid %d: passed %v, log %q", client.family, st, uid, passed, log)
				}
			}
		}
	}
}

// TestUserListenerRefusesAServerThatIsNotEstablished: once the server's socket
// has seen a FIN or a reset, the client is gone, whatever the table says of it.
func TestUserListenerRefusesAServerThatIsNotEstablished(t *testing.T) {
	servers := []struct{ name, row string }{{"missing", ""}}
	for st := 0x01; st <= 0x0C; st++ {
		if st != established {
			servers = append(servers, struct{ name, row string }{fmt.Sprintf("in state %02X", st), serverRow(st)})
		}
	}
	clients := []struct{ name, row string }{
		{"has no row", ""},
		{"is the user's own", clientRow(established, ownUID)},
		{"is root", clientRow(established, 0)},
		{"is the user's own over IPv6", clientRow6(established, ownUID)},
		{"is in FIN_WAIT2", clientRow(finWait2, 0)},
		{"is in FIN_WAIT1", clientRow(finWait1, ownUID)},
		{"is in TIME_WAIT over IPv6", clientRow6(timeWait, 0)},
	}
	for _, server := range servers {
		for _, client := range clients {
			if passed, log := accept(t, server.row+client.row); passed || log != closedLog {
				t.Errorf("server %s, client %s: passed %v, log %q", server.name, client.name, passed, log)
			}
		}
	}
}

// TestUserListenerChecksTheServerAfterTheClient: reading the tables takes
// milliseconds and gives no snapshot. A client that resets while they are read
// can have no row in the text although the server's row, read earlier, still
// says ESTABLISHED, which would pass it as a relay. So the server's row is read
// again after the client's lookup, and by then it shows the reset.
func TestUserListenerChecksTheServerAfterTheClient(t *testing.T) {
	tests := []struct {
		name   string
		reads  []string // the lines of each read of the tables; the last one repeats
		passes bool
	}{
		{"the client's row is gone and the server has seen the reset",
			[]string{serverRow(established), ""}, false},
		{"the client's row is gone and the server's socket is closing",
			[]string{serverRow(established), serverRow(closeWait)}, false},
		{"the client closes after the lookup",
			[]string{serverRow(established) + clientRow(established, ownUID), serverRow(closeWait) + clientRow(finWait2, 0)}, false},
		{"the connection stays",
			[]string{serverRow(established) + clientRow(established, ownUID), serverRow(established) + clientRow(established, ownUID)}, true},
		{"a relay",
			[]string{serverRow(established), serverRow(established)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := func() ([]byte, error) {
				text := tableHeader + tt.reads[0]
				if len(tt.reads) > 1 {
					tt.reads = tt.reads[1:]
				}
				return []byte(text), nil
			}
			passed, log := acceptFrom(t, table)
			want := closedLog
			if tt.passes {
				want = ""
			}
			if passed != tt.passes || log != want {
				t.Errorf("passed %v, log %q; want %v, %q", passed, log, tt.passes, want)
			}
		})
	}
}

func TestUserListenerRefusesWhenTheTableCannotBeRead(t *testing.T) {
	passed, log := acceptFrom(t, func() ([]byte, error) { return nil, errors.New("too many open files") })
	if passed || log != closedLog {
		t.Errorf("passed %v, log %q", passed, log)
	}
}

func TestUserListener(t *testing.T) {
	// The tables list the sockets of all the connections: each has a client
	// port of its own.
	var rows strings.Builder
	var conns []*fakeConn
	add := func(serverState, clientState, uid int) *fakeConn {
		client := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5000 + len(conns)}
		if serverState != 0 {
			rows.WriteString(tableRow(serverAddr, client, serverState, ownUID))
		}
		if clientState != 0 {
			rows.WriteString(tableRow(client, serverAddr, clientState, uid))
		}
		c := &fakeConn{client: client}
		conns = append(conns, c)
		return c
	}
	refused := []*fakeConn{
		add(established, established, otherUID),
		add(established, established, otherUID),
		add(closeWait, finWait2, 0),
		add(0, 0, 0),
		add(established, established, otherUID+1),
	}
	root := add(established, established, 0)
	own := add(established, established, ownUID)

	log := &lockedBuffer{}
	table := func() ([]byte, error) { return []byte(tableHeader + rows.String()), nil }
	l := guardOf(append([]*fakeConn(nil), conns...), table, log)
	for _, want := range []*fakeConn{root, own} {
		c, err := l.Accept()
		if err != nil || c != net.Conn(want) {
			t.Fatalf("Accept = %v, %v", c, err)
		}
	}
	for i, c := range refused {
		if !c.closed {
			t.Errorf("connection %d was not closed", i)
		}
	}
	if root.closed || own.closed {
		t.Errorf("a connection that passed was closed: %v %v", root.closed, own.closed)
	}
	// One line for each other user, and one for the connections that closed.
	if want := otherLog + closedLog + "graft ui: refused a connection from uid 4243\n"; log.String() != want {
		t.Errorf("log %q, want %q", log.String(), want)
	}
}

func TestUserListenerPassesAnUnknownSocket(t *testing.T) {
	// The client's socket is in neither table, and the server's is established.
	if passed, log := accept(t, serverRow(established)); !passed || log != "" {
		t.Errorf("passed %v, log %q", passed, log)
	}
}
