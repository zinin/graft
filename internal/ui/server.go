// Package ui serves graft ui: a page in the browser that edits the config
// file, on 127.0.0.1, for the user who started it.
package ui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
)

// csp lets the page load only its own files and talk only to its server;
// data: is for the icons of Pico's stylesheet.
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// maxBody is the largest request body: a config is a few kilobytes.
const maxBody = 1 << 20

//go:embed static
var static embed.FS

// Options configure a Server.
type Options struct {
	Path string    // the config file, absolute
	Log  io.Writer // the terminal: a line for each write and each refused user; safe for concurrent use
	// Explicit says that Path came from --config, so graft needs --config
	// too to read this file.
	Explicit bool
	// What a profile would start with, for the key widget, the test request
	// and Preview: the environment of graft ui, its home directory and the
	// directory it runs in, whose .claude settings Preview checks.
	Environ []string
	Home    string
	Dir     string
	// LookPath finds claude for Preview; exec.LookPath outside tests.
	LookPath func(string) (string, error)
	// Stdin and Stderr are the terminal of graft ui, where a key command may
	// ask for a password.
	Stdin  io.Reader
	Stderr io.Writer
}

// Server is the HTTP server of graft ui.
type Server struct {
	http  *http.Server
	ln    net.Listener
	port  int
	token string
}

// New opens the server's socket on 127.0.0.1 at a random port. Serve
// serves it.
func New(opts Options) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		ln.Close()
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	files, err := fs.Sub(static, "static")
	if err != nil {
		ln.Close()
		return nil, err
	}
	mux := http.NewServeMux()
	newAPI(opts).routes(mux)
	mux.Handle("GET /", http.FileServerFS(files))
	return &Server{
		http:  &http.Server{Handler: guard(mux, port, token), ReadHeaderTimeout: 10 * time.Second},
		ln:    userGuard(ln, opts.Log),
		port:  port,
		token: token,
	}, nil
}

// URL is the address of the page with the token, which the page moves out
// of the address bar.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/#token=%s", s.port, s.token)
}

// Port is the server's port on 127.0.0.1.
func (s *Server) Port() int { return s.port }

// Serve serves until Shutdown.
func (s *Server) Serve() error {
	if err := s.http.Serve(s.ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the server, waiting for the requests in progress until
// ctx ends.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// guard lets through only requests for this server from its own page:
// Host must be 127.0.0.1 or localhost at port, which stops DNS rebinding;
// Origin, when present, must be the page's own; and /api/ needs the token.
// It also sets the headers every response carries.
func guard(next http.Handler, port int, token string) http.Handler {
	hosts := []string{fmt.Sprintf("127.0.0.1:%d", port), fmt.Sprintf("localhost:%d", port)}
	origins := []string{"http://" + hosts[0], "http://" + hosts[1]}
	auth := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if !slices.Contains(hosts, r.Host) {
			http.Error(w, "graft ui: this server answers only at "+hosts[0], http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !slices.Contains(origins, o) {
			http.Error(w, "graft ui: requests from other pages are refused", http.StatusForbidden)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-cache")
			next.ServeHTTP(w, r)
			return
		}
		h.Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), auth) != 1 {
			writeJSON(w, http.StatusUnauthorized, errorBody{"graft ui: missing or wrong token: open the address that graft ui printed"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}
