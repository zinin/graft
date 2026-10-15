package ui

import (
	"net"
	"os"
	"testing"
)

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
	if c := <-accepted; c == nil {
		t.Fatal("own connection refused")
	}
	table, _ := os.ReadFile("/proc/net/tcp")
	uid, found := socketUID(table, conn.LocalAddr().(*net.TCPAddr), conn.RemoteAddr().(*net.TCPAddr))
	if !found || uid != os.Getuid() {
		t.Errorf("socketUID = %d, %v; want %d", uid, found, os.Getuid())
	}
}
