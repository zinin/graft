package ui

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

// request is what a client writes before it closes its socket: a save, which
// would change the config if the server served it.
const request = "POST /api/save HTTP/1.1\r\nHost: 127.0.0.1\r\nContent-Length: 0\r\n\r\n"

// listenLocal opens a listener on 127.0.0.1, as Server does.
func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

// acceptAsync calls l.Accept in a goroutine, so that a test can make the
// connections it waits for after the call. The channel gets nil if Accept fails.
func acceptAsync(l net.Listener) <-chan net.Conn {
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := l.Accept()
		accepted <- c
	}()
	return accepted
}

// wantOpenClient checks that Accept returned the server's end of the
// connection from open, which was made after the connections that the guard
// should have refused.
func wantOpenClient(t *testing.T, accepted <-chan net.Conn, open net.Conn) {
	t.Helper()
	select {
	case c := <-accepted:
		if c == nil {
			t.Fatal("Accept failed")
		}
		t.Cleanup(func() { c.Close() })
		if got, want := c.RemoteAddr().String(), open.LocalAddr().String(); got != want {
			t.Errorf("Accept returned the connection from %s, want the open one from %s", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Accept did not return the open connection")
	}
}

// kernelRow looks in the real table, /proc/net/tcp or /proc/net/tcp6, for the
// socket at local that is connected to remote.
func kernelRow(t *testing.T, file, local, remote string) (socketRow, bool) {
	t.Helper()
	table, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return findSocket(table, local, remote)
}

// waitUntil polls cond, because the kernel changes the state of a socket a
// little after the call that caused it.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("gave up waiting for %s", what)
}

// dialMapped connects an AF_INET6 socket to ::ffff:127.0.0.1 at port, which
// reaches an IPv4 listener. net.Dial would use an AF_INET socket for that
// address. The test is skipped on a machine without IPv6 sockets. It returns
// the addresses of the socket's two ends.
func dialMapped(t *testing.T, port int) (local, remote *net.TCPAddr) {
	t.Helper()
	fd, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Skipf("no IPv6 sockets here: %v", err)
	}
	t.Cleanup(func() { syscall.Close(fd) })
	// Some systems make an IPv6 socket IPv6-only by default, which refuses
	// IPv4-mapped addresses.
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 0); err != nil {
		t.Fatal(err)
	}
	remoteAddr := &syscall.SockaddrInet6{Port: port}
	copy(remoteAddr.Addr[:], net.ParseIP("::ffff:127.0.0.1"))
	if err := syscall.Connect(fd, remoteAddr); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.Getsockname(fd)
	if err != nil {
		t.Fatal(err)
	}
	self := name.(*syscall.SockaddrInet6)
	return &net.TCPAddr{IP: self.Addr[:], Port: self.Port}, &net.TCPAddr{IP: remoteAddr.Addr[:], Port: port}
}

// TestUserGuardFindsOwnSocket checks the real /proc/net/tcp: a connection
// from this process is found, belongs to this user, and passes.
func TestUserGuardFindsOwnSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	guarded := userGuard(ln, &lockedBuffer{})
	defer guarded.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := guarded.Accept()
		accepted <- c
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := <-accepted
	if c == nil {
		t.Fatal("own connection refused")
	}
	// The server's end stays open: a garbage collection that closed it would
	// make the kernel list the client as CLOSE_WAIT.
	defer c.Close()
	table, _ := os.ReadFile("/proc/net/tcp")
	local, remote := conn.LocalAddr().(*net.TCPAddr), conn.RemoteAddr().(*net.TCPAddr)
	row, found := findSocket(table, procAddr(local), procAddr(remote))
	if !found || row.uid != os.Getuid() || row.state != established {
		t.Errorf("findSocket = %+v, %v; want an established socket of user %d", row, found, os.Getuid())
	}
}

// TestUserGuardRefusesAClientThatClosedBeforeAccept: a client that writes its
// request and closes before the server's Accept has its request in the server
// socket's buffer. A little after the close, the kernel lists the client as a
// time-wait socket of uid 0, which the guard once passed as root.
func TestUserGuardRefusesAClientThatClosedBeforeAccept(t *testing.T) {
	ln := listenLocal(t)
	log := &lockedBuffer{}
	guarded := userGuard(ln, log)

	early, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	local, remote := early.LocalAddr().(*net.TCPAddr), early.RemoteAddr().(*net.TCPAddr)
	if _, err := early.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	early.Close()
	// The row is FIN_WAIT1, of the real user, until the server's kernel
	// acknowledges the FIN, which it may delay by some 40 ms.
	waitUntil(t, "the kernel to list the closed client as FIN_WAIT2", func() bool {
		row, found := kernelRow(t, "/proc/net/tcp", procAddr(local), procAddr(remote))
		return found && row.state == finWait2
	})

	accepted := acceptAsync(guarded)
	open, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer open.Close()
	wantOpenClient(t, accepted, open)
	if got := log.String(); got != closedLog {
		t.Errorf("log %q, want %q", got, closedLog)
	}
}

// TestUserGuardRefusesAClientThatResetBeforeAccept: a client that resets its
// socket after writing its request leaves no row in the table, and its request
// is still readable by the server.
func TestUserGuardRefusesAClientThatResetBeforeAccept(t *testing.T) {
	ln := listenLocal(t)
	log := &lockedBuffer{}
	guarded := userGuard(ln, log)

	early, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	local, remote := early.LocalAddr().(*net.TCPAddr), early.RemoteAddr().(*net.TCPAddr)
	if _, err := early.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	early.(*net.TCPConn).SetLinger(0)
	early.Close()
	waitUntil(t, "the kernel to drop both sockets of the reset connection", func() bool {
		_, clientFound := kernelRow(t, "/proc/net/tcp", procAddr(local), procAddr(remote))
		server, serverFound := kernelRow(t, "/proc/net/tcp", procAddr(remote), procAddr(local))
		return !clientFound && (!serverFound || server.state != established)
	})

	accepted := acceptAsync(guarded)
	open, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer open.Close()
	wantOpenClient(t, accepted, open)
	if got := log.String(); got != closedLog {
		t.Errorf("log %q, want %q", got, closedLog)
	}
}

// TestUserGuardFindsAnIPv6Client: an AF_INET6 socket connected to
// ::ffff:127.0.0.1 reaches the IPv4 listener, but the kernel lists the socket
// in /proc/net/tcp6 only, with IPv4-mapped addresses.
func TestUserGuardFindsAnIPv6Client(t *testing.T) {
	ln := listenLocal(t)
	guarded := userGuard(ln, &lockedBuffer{})
	accepted := acceptAsync(guarded)
	local, remote := dialMapped(t, ln.Addr().(*net.TCPAddr).Port)

	select {
	case c := <-accepted:
		if c == nil {
			t.Fatal("own connection refused")
		}
		// The connection stays open: the kernel lists the client as
		// ESTABLISHED only while the server has not closed its end.
		defer c.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("Accept did not return the connection")
	}
	row, found := kernelRow(t, "/proc/net/tcp6", procAddr6(local), procAddr6(remote))
	if !found || row.uid != os.Getuid() || row.state != established {
		t.Errorf("findSocket in tcp6 = %+v, %v; want an established socket of user %d", row, found, os.Getuid())
	}
}

// TestUserGuardRefusesAnIPv6ClientOfAnotherUser builds the guard for a user
// other than the one that runs the test, so that a client from this process is
// another user's. An IPv6 client must be found in tcp6 and refused, not passed
// as a relay.
func TestUserGuardRefusesAnIPv6ClientOfAnotherUser(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root passes every guard")
	}
	ln := listenLocal(t)
	log := &lockedBuffer{}
	guarded := &userListener{Listener: ln, uid: os.Getuid() + 1, table: procTables, log: log, refused: map[int]bool{}}
	accepted := acceptAsync(guarded)
	dialMapped(t, ln.Addr().(*net.TCPAddr).Port)

	want := fmt.Sprintf("graft ui: refused a connection from uid %d\n", os.Getuid())
	waitUntil(t, "the refusal", func() bool {
		select {
		case c := <-accepted:
			t.Fatalf("Accept returned %v: the IPv6 client of another user passed", c)
		default:
		}
		return log.String() != ""
	})
	guarded.Close()
	if c := <-accepted; c != nil {
		t.Errorf("Accept returned %v after the refusal", c)
	}
	if got := log.String(); got != want {
		t.Errorf("log %q, want %q", got, want)
	}
}
