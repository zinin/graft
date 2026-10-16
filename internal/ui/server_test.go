package ui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// testServer is a running Server for a config file in a temporary
// directory.
type testServer struct {
	*Server
	log *lockedBuffer
}

func startServer(t *testing.T, content string) *testServer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	log := &lockedBuffer{}
	s, err := New(Options{Path: path, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	t.Cleanup(func() { s.Shutdown(t.Context()) })
	return &testServer{s, log}
}

func (s *testServer) base() string {
	return strings.Split(s.URL(), "/#")[0]
}

func TestURL(t *testing.T) {
	s := startServer(t, testConfig)
	if !regexp.MustCompile(`^http://127\.0\.0\.1:\d+/#token=[0-9a-f]{64}$`).MatchString(s.URL()) {
		t.Errorf("URL %q", s.URL())
	}
}

func TestGuard(t *testing.T) {
	s := startServer(t, testConfig)
	tests := []struct {
		name   string
		path   string
		host   string // "" is the server's own
		origin string
		auth   string // "" is the right token
		code   int
	}{
		{"page", "/", "", "", "none", 200},
		{"api", "/api/version", "", "", "", 200},
		{"no token", "/api/version", "", "", "none", 401},
		{"wrong token", "/api/version", "", "", "Bearer 0000", 401},
		{"localhost", "/api/version", "localhost:%d", "", "", 200},
		{"rebinding", "/api/version", "evil.example:%d", "", "", 403},
		{"rebinding to the page", "/", "evil.example:%d", "", "none", 403},
		{"other port", "/api/version", "127.0.0.1:1", "", "", 403},
		{"own origin", "/api/version", "", "http://127.0.0.1:%d", "", 200},
		{"other origin", "/api/version", "", "https://evil.example", "", 403},
	}
	for _, tt := range tests {
		req, _ := http.NewRequest("GET", s.base()+tt.path, nil)
		if tt.host != "" {
			req.Host = fmt.Sprintf(tt.host, s.Port())
		}
		if tt.origin != "" {
			req.Header.Set("Origin", fmt.Sprintf(tt.origin, s.Port()))
		}
		switch tt.auth {
		case "":
			req.Header.Set("Authorization", "Bearer "+s.token)
		case "none":
		default:
			req.Header.Set("Authorization", tt.auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tt.code {
			t.Errorf("%s: %d, want %d", tt.name, resp.StatusCode, tt.code)
		}
		if got := resp.Header.Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self'") {
			t.Errorf("%s: CSP %q", tt.name, got)
		}
	}
}

func TestHeaders(t *testing.T) {
	s := startServer(t, testConfig)
	get := func(path string, auth bool) *http.Response {
		req, _ := http.NewRequest("GET", s.base()+path, nil)
		if auth {
			req.Header.Set("Authorization", "Bearer "+s.token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp
	}
	page := get("/", false)
	if page.Header.Get("Cache-Control") != "no-cache" || page.Header.Get("X-Content-Type-Options") != "nosniff" ||
		page.Header.Get("Referrer-Policy") != "no-referrer" || !strings.HasPrefix(page.Header.Get("Content-Type"), "text/html") {
		t.Errorf("page headers %v", page.Header)
	}
	if api := get("/api/version", true); api.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("api headers %v", api.Header)
	}
}

func TestBodyLimit(t *testing.T) {
	s := startServer(t, testConfig)
	big := `{"version": "` + strings.Repeat("a", maxBody) + `"}`
	req, _ := http.NewRequest("POST", s.base()+"/api/validate", strings.NewReader(big))
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Errorf("big body: %d, want 413", resp.StatusCode)
	}
}
