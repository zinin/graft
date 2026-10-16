package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// provider is a fake provider: it answers each request with status and
// body, and keeps the last request.
type provider struct {
	*httptest.Server
	status int
	body   string
	calls  int
	last   *http.Request
	sent   map[string]any // the JSON body of the last request
}

func startProvider(t *testing.T, status int, body string) *provider {
	t.Helper()
	p := &provider{status: status, body: body}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.calls++
		p.last = r
		p.sent = nil
		json.NewDecoder(r.Body).Decode(&p.sent)
		if p.status/100 == 3 {
			w.Header().Set("Location", "http://elsewhere.invalid/")
		}
		w.WriteHeader(p.status)
		io.WriteString(w, p.body)
	}))
	t.Cleanup(p.Close)
	return p
}

// sendTestRequest asks the API to test model id at url with the key of the
// variable TEST_KEY, sk-test.
func sendTestRequest(t *testing.T, s *testAPI, url, id string, out any) int {
	t.Helper()
	return s.call(t, "POST", "/api/test", map[string]any{"url": url, "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": id}, out)
}

func testEnv() Options { return Options{Environ: []string{"TEST_KEY=sk-test"}} }

func TestTestRequest(t *testing.T) {
	p := startProvider(t, 200, `{"type":"message","content":[{"type":"thinking","thinking":"hm"},{"type":"text","text":"pong"}]}`)
	s := startWith(t, "", testEnv())
	var res testResult
	if code := sendTestRequest(t, s, p.URL+"/anthropic/", "glm-5.3-flash", &res); code != 200 {
		t.Fatalf("POST /api/test: %d", code)
	}
	if res.Status != 200 || res.Text != "pong" || res.Body != "" || res.MS < 0 {
		t.Errorf("result %+v", res)
	}
	r := p.last
	if r.Method != "POST" || r.URL.Path != "/anthropic/v1/messages" {
		t.Errorf("request %s %s, want POST /anthropic/v1/messages", r.Method, r.URL.Path)
	}
	for name, want := range map[string]string{"Authorization": "Bearer sk-test", "Anthropic-Version": "2023-06-01", "Content-Type": "application/json"} {
		if got := r.Header.Get(name); got != want {
			t.Errorf("header %s: %q, want %q", name, got, want)
		}
	}
	want := map[string]any{
		"model":      "glm-5.3-flash",
		"max_tokens": float64(16),
		"messages":   []any{map[string]any{"role": "user", "content": "Reply with one word: pong"}},
	}
	if !reflect.DeepEqual(p.sent, want) {
		t.Errorf("body %v, want %v", p.sent, want)
	}
}

func TestTestRequestJoinsTheURL(t *testing.T) {
	p := startProvider(t, 200, `{"content":[]}`)
	s := startWith(t, "", testEnv())
	for base, path := range map[string]string{"": "/v1/messages", "/": "/v1/messages", "/api": "/api/v1/messages"} {
		if code := sendTestRequest(t, s, p.URL+base, "m", nil); code != 200 || p.last.URL.Path != path {
			t.Errorf("url %q: %d, path %q, want %q", base, code, p.last.URL.Path, path)
		}
	}
}

// TestTestRequestAnswers: the page gets the status and some of the answer,
// never the key.
func TestTestRequestAnswers(t *testing.T) {
	long := strings.Repeat("é", 300)
	tests := []struct {
		name   string
		status int
		body   string
		want   testResult
	}{
		{"a model that only thinks", 200, `{"content":[{"type":"thinking","thinking":"..."}]}`, testResult{Status: 200}},
		{"a long answer", 200, `{"content":[{"type":"text","text":"sk-test ` + long + `"}]}`,
			testResult{Status: 200, Text: "<key> " + strings.Repeat("é", 194)}},
		{"a refused key", 401, `{"error":{"message":"invalid key sk-test"}}`,
			testResult{Status: 401, Body: `{"error":{"message":"invalid key <key>"}}`}},
		{"a long error", 500, "x" + strings.Repeat("é", 1500), testResult{Status: 500, Body: "x" + strings.Repeat("é", 1023)}},
		// Cut first, the body would end in sk-t, the start of the key.
		{"a key across the cut", 401, strings.Repeat("x", 2044) + "sk-test", testResult{Status: 401, Body: strings.Repeat("x", 2044) + "<key"}},
		{"a redirect", 307, "", testResult{Status: 307}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := startProvider(t, tt.status, tt.body)
			s := startWith(t, "", testEnv())
			var res testResult
			if code := sendTestRequest(t, s, p.URL, "m", &res); code != 200 {
				t.Fatalf("POST /api/test: %d", code)
			}
			res.MS = 0
			if res != tt.want {
				t.Errorf("result %+v, want %+v", res, tt.want)
			}
			if !utf8.ValidString(res.Body) || len(res.Body) > 2048 || utf8.RuneCountInString(res.Text) > 200 {
				t.Errorf("text %d characters, body %d bytes", utf8.RuneCountInString(res.Text), len(res.Body))
			}
			if p.calls != 1 {
				t.Errorf("%d requests, want 1: no retry and no redirect", p.calls)
			}
		})
	}
}

func TestTestRequestRefusals(t *testing.T) {
	s := startWith(t, "", testEnv())
	dir := t.TempDir()
	cmd := "cat '" + filepath.Join(dir, "key") + "'"
	tests := []struct {
		name string
		req  map[string]any
		code int
		msg  string
	}{
		{"not http", map[string]any{"url": "ftp://x", "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": "m"},
			422, "url must be an absolute http or https URL"},
		{"a query", map[string]any{"url": "https://x/?a=1", "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": "m"},
			422, "url must not have a query or fragment: Claude Code appends /v1/messages to it"},
		{"no id", map[string]any{"url": "https://x", "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": ""},
			422, "the model needs an id or a name, without white space"},
		{"no key", map[string]any{"url": "https://x", "key": map[string]string{"kind": "env", "value": "MISSING"}, "id": "m"},
			422, "env MISSING is not set"},
		{"an empty source", map[string]any{"url": "https://x", "key": map[string]string{"kind": "file", "value": ""}, "id": "m"},
			422, "key: must not be empty"},
		{"a command not run", map[string]any{"url": "https://x", "key": map[string]string{"kind": "command", "value": cmd}, "id": "m"},
			422, errNotRun.Error()},
		{"no provider", map[string]any{"url": "http://127.0.0.1:1", "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": "m"},
			502, "connection refused"},
	}
	for _, tt := range tests {
		var body errorBody
		code := s.call(t, "POST", "/api/test", tt.req, &body)
		if code != tt.code || !strings.Contains(body.Error, tt.msg) {
			t.Errorf("%s: %d %q, want %d %q", tt.name, code, body.Error, tt.code, tt.msg)
		}
	}
}

// TestTestRequestUsesTheKeptKey: the test asks no password again.
func TestTestRequestUsesTheKeptKey(t *testing.T) {
	p := startProvider(t, 200, `{"content":[{"type":"text","text":"pong"}]}`)
	s := startAPI(t, "")
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	cmd := "echo run >> '" + count + "'; echo sk-cmd"
	s.checkKey(t, "command", cmd, true)
	req := map[string]any{"url": p.URL, "key": map[string]string{"kind": "command", "value": cmd}, "id": "m"}
	if code := s.call(t, "POST", "/api/test", req, nil); code != 200 {
		t.Fatalf("POST /api/test: %d", code)
	}
	if got := p.last.Header.Get("Authorization"); got != "Bearer sk-cmd" {
		t.Errorf("Authorization %q", got)
	}
	if data, _ := os.ReadFile(count); string(data) != "run\n" {
		t.Errorf("the command ran %d times, want once", strings.Count(string(data), "run"))
	}
}

func TestTestRequestTimesOut(t *testing.T) {
	defer func(d time.Duration) { testTimeout = d }(testTimeout)
	testTimeout = 100 * time.Millisecond
	release := make(chan struct{})
	p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer p.Close()
	defer close(release)
	s := startWith(t, "", testEnv())
	var body errorBody
	if code := sendTestRequest(t, s, p.URL, "m", &body); code != 502 || !strings.Contains(body.Error, "Client.Timeout exceeded") {
		t.Errorf("%d %q, want 502 and a timeout", code, body.Error)
	}
}
