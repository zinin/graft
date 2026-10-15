package ui

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

const tableHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

// tableRow is a line of /proc/net/tcp for a socket owned by uid.
func tableRow(local, remote *net.TCPAddr, uid int) string {
	return fmt.Sprintf("   0: %s %s 01 00000000:00000000 00:00000000 00000000 %5d        0 1 1 0000000000000000 20 4 30 10 -1\n",
		procAddr(local), procAddr(remote), uid)
}

var (
	serverAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8000}
	clientAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5000}
)

func TestSocketUID(t *testing.T) {
	table := []byte(tableHeader + tableRow(serverAddr, clientAddr, 1000) + tableRow(clientAddr, serverAddr, 1001))
	if uid, found := socketUID(table, clientAddr, serverAddr); !found || uid != 1001 {
		t.Errorf("client socket: %d, %v", uid, found)
	}
	other := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5001}
	if _, found := socketUID(table, other, serverAddr); found {
		t.Error("found a socket that is not in the table")
	}
}

func TestProcAddrIsLittleEndianOnThisMachine(t *testing.T) {
	// graft is built for amd64 and arm64, both little-endian.
	if got := procAddr(clientAddr); got != "0100007F:1388" {
		t.Errorf("procAddr = %q", got)
	}
}

type fakeConn struct {
	net.Conn
	closed bool
}

func (c *fakeConn) LocalAddr() net.Addr  { return serverAddr }
func (c *fakeConn) RemoteAddr() net.Addr { return clientAddr }
func (c *fakeConn) Close() error         { c.closed = true; return nil }

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

func TestUserListener(t *testing.T) {
	owners := []int{4242, 4242, 0, 1000}
	conns := []*fakeConn{{}, {}, {}, {}}
	log := &lockedBuffer{}
	l := &userListener{
		Listener: &fakeListener{conns: append([]*fakeConn(nil), conns...)},
		uid:      1000,
		table: func() ([]byte, error) {
			uid := owners[0]
			owners = owners[1:]
			return []byte(tableHeader + tableRow(clientAddr, serverAddr, uid)), nil
		},
		log:     log,
		refused: map[int]bool{},
	}
	for _, want := range []*fakeConn{conns[2], conns[3]} {
		c, err := l.Accept()
		if err != nil || c != want {
			t.Fatalf("Accept = %v, %v", c, err)
		}
	}
	if !conns[0].closed || !conns[1].closed || conns[2].closed || conns[3].closed {
		t.Errorf("closed: %v %v %v %v", conns[0].closed, conns[1].closed, conns[2].closed, conns[3].closed)
	}
	if got := log.String(); got != "graft ui: refused a connection from uid 4242\n" {
		t.Errorf("log %q", got)
	}
}

func TestUserListenerPassesAnUnknownSocket(t *testing.T) {
	c := &fakeConn{}
	l := &userListener{
		Listener: &fakeListener{conns: []*fakeConn{c}},
		uid:      1000,
		table:    func() ([]byte, error) { return []byte(tableHeader), nil },
		log:      &strings.Builder{},
		refused:  map[int]bool{},
	}
	if got, err := l.Accept(); got != c || err != nil {
		t.Errorf("Accept = %v, %v", got, err)
	}
}
