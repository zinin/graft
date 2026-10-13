package router

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer is a buffer that handlers may write while a test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

var stamp = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ `)

// lines returns the lines of a log without their time stamps; it fails the
// test when a line lacks one.
func lines(t *testing.T, log string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(strings.TrimSuffix(log, "\n"), "\n") {
		if line == "" {
			continue
		}
		if !stamp.MatchString(line) {
			t.Errorf("line %q has no time stamp in UTC", line)
		}
		out = append(out, stamp.ReplaceAllString(line, ""))
	}
	return out
}

func TestLogLines(t *testing.T) {
	var b lockedBuffer
	l := NewLog(&b, "3f9a1c")
	l.Printf("start profile=%s port=%d", "mix", 43117)
	l.request(&request{method: "POST", path: "/v1/messages", model: "glm", route: "ollama", status: 200,
		dur: 12400 * time.Millisecond, bytes: 48213})
	l.request(&request{method: "POST", path: "/v1/messages", model: "sol", route: "cliproxy",
		dur: 3 * time.Second, canceled: true})
	l.request(&request{method: "POST", path: "/v1/messages", model: "sol", route: "cliproxy", status: 502,
		bytes: 120, err: "dial tcp: connection refused"})
	l.request(&request{method: "HEAD", path: "/api/hello", route: "graft", status: 200})
	l.request(&request{method: "GET", path: "/a b\nforged line", model: "x y", route: "graft", status: 404, bytes: 90})
	want := []string{
		`session=3f9a1c start profile=mix port=43117`,
		`session=3f9a1c POST /v1/messages model=glm route=ollama status=200 dur=12.4s bytes=48213`,
		`session=3f9a1c POST /v1/messages model=sol route=cliproxy status=0 dur=3.0s bytes=0 canceled=true`,
		`session=3f9a1c POST /v1/messages model=sol route=cliproxy status=502 dur=0.0s bytes=120 err="dial tcp: connection refused"`,
		`session=3f9a1c HEAD /api/hello route=graft status=200 dur=0.0s bytes=0`,
		`session=3f9a1c GET "/a b\nforged line" model="x y" route=graft status=404 dur=0.0s bytes=90`,
	}
	got := lines(t, b.String())
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("log:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestNilLog(t *testing.T) {
	if NewLog(nil, "3f9a1c") != nil {
		t.Error("NewLog(nil) is not nil")
	}
	var l *Log
	l.Printf("start")
	l.request(&request{method: "GET", path: "/"})
	l.logger().Print("lost")
}
