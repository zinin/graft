package router

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Log writes graft's log lines, each with the time in UTC and the session.
// The log holds no headers, bodies, keys or secret. A nil *Log writes
// nothing.
type Log struct {
	mu      sync.Mutex
	w       io.Writer
	session string
}

// NewLog returns a log that writes to w; with a nil w it returns nil.
func NewLog(w io.Writer, session string) *Log {
	if w == nil {
		return nil
	}
	return &Log{w: w, session: session}
}

// Printf writes one line.
func (l *Log) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	line := fmt.Sprintf("%s session=%s %s\n", time.Now().UTC().Format(time.RFC3339), l.session, fmt.Sprintf(format, args...))
	l.mu.Lock()
	defer l.mu.Unlock()
	io.WriteString(l.w, line)
}

// request is a finished request as its log line shows it.
type request struct {
	method, path string
	model        string // empty when the request names none
	route        string // the provider, anthropic, or graft when graft answered itself
	status       int    // 0 when no answer went out
	dur          time.Duration
	bytes        int64 // of the response body
	canceled     bool  // the client went away
	err          string
}

func (l *Log) request(q *request) {
	if l == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", q.method, field(q.path))
	if q.model != "" {
		fmt.Fprintf(&b, " model=%s", field(q.model))
	}
	fmt.Fprintf(&b, " route=%s status=%d dur=%.1fs bytes=%d", q.route, q.status, q.dur.Seconds(), q.bytes)
	if q.canceled {
		b.WriteString(" canceled=true")
	}
	if q.err != "" {
		fmt.Fprintf(&b, " err=%q", q.err)
	}
	l.Printf("%s", b.String())
}

// plain matches what a log line shows as it is. Anything else is quoted, so
// that no request can break a line or forge one.
var plain = regexp.MustCompile(`^[A-Za-z0-9._~:/@+-]+$`)

func field(s string) string {
	if plain.MatchString(s) {
		return s
	}
	return strconv.Quote(s)
}

// logger returns a standard logger whose lines go to l: net/http and
// ReverseProxy must not write to the terminal, which belongs to claude.
func (l *Log) logger() *log.Logger {
	if l == nil {
		return log.New(io.Discard, "", 0)
	}
	return log.New(logWriter{l}, "", 0)
}

type logWriter struct{ l *Log }

func (w logWriter) Write(p []byte) (int, error) {
	w.l.Printf("%s", strings.TrimSuffix(string(p), "\n"))
	return len(p), nil
}

// recorder notes what goes back to the client: the status, the size of the
// body, and whether a write failed, which means the client has gone.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	failed bool
}

func (rec *recorder) WriteHeader(code int) {
	if rec.status == 0 && code >= 200 {
		rec.status = code
	}
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *recorder) Write(p []byte) (int, error) {
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	n, err := rec.ResponseWriter.Write(p)
	rec.bytes += int64(n)
	if err != nil {
		rec.failed = true
	}
	return n, err
}

// Unwrap lets http.ResponseController reach the real writer, which is how
// ReverseProxy flushes each piece of a stream.
func (rec *recorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
}
