package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
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
// variable TEST_KEY, which testEnv sets to sk-test.
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
	tests := []struct{ name, base, path, uri string }{
		{"empty", "", "/v1/messages", "/v1/messages"},
		{"root", "/", "/v1/messages", "/v1/messages"},
		{"path", "/api", "/api/v1/messages", "/api/v1/messages"},
		{"trailing slash", "/api/", "/api/v1/messages", "/api/v1/messages"},
		{"two trailing slashes", "/api//", "/api//v1/messages", "/api//v1/messages"},
		{"escaped separator", "/api%2Fproxy", "/api/proxy/v1/messages", "/api%2Fproxy/v1/messages"},
		{"lower-case escape", "/api%2fproxy", "/api/proxy/v1/messages", "/api%2fproxy/v1/messages"},
		{"escaped trailing slash", "/api%2F", "/api//v1/messages", "/api%2F/v1/messages"},
		{"escaped then literal slash", "/api%2f/", "/api//v1/messages", "/api%2f/v1/messages"},
		{"double escape", "/api%252Fproxy", "/api%2Fproxy/v1/messages", "/api%252Fproxy/v1/messages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code := sendTestRequest(t, s, p.URL+tt.base, "m", nil); code != 200 {
				t.Fatalf("url %q: status %d, want 200", tt.base, code)
			}
			if p.last.URL.Path != tt.path {
				t.Errorf("url %q: path %q, want %q", tt.base, p.last.URL.Path, tt.path)
			}
			// The wire path distinguishes an escaped separator from a slash,
			// even when the upstream decodes both to the same URL.Path.
			if p.last.RequestURI != tt.uri {
				t.Errorf("url %q: RequestURI %q, want %q", tt.base, p.last.RequestURI, tt.uri)
			}
		})
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
		// JSON turns each byte that is not UTF-8 into U+FFFD, three bytes: made
		// valid only after the cut, the body would reach the page twice as long
		// as 2 KiB.
		{"a binary error", 500, strings.Repeat("x\xff", 1500), testResult{Status: 500, Body: strings.Repeat("x"+string(utf8.RuneError), 512)}},
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

// escapedKey holds characters that JSON encoders escape, each in its own way.
const escapedKey = "sk-a\"b\\c/d<e>f&g+h'i"

// TestTestRequestRedactsAnEscapedKey: the page gets <key> for a key that the
// provider repeats as its JSON writes it, also across the cut.
func TestTestRequestRedactsAnEscapedKey(t *testing.T) {
	refused, _ := json.Marshal(map[string]any{"error": map[string]any{"message": "invalid key " + escapedKey}})
	spelled, _ := json.Marshal(escapedKey) // in quotes, which the body leaves out
	tests := []struct {
		name string
		body string
		want string
	}{
		{"a refused key", string(refused), `{"error":{"message":"invalid key <key>"}}`},
		// Cut first or redacted only as it is, the body would end in the
		// start of the key.
		{"a key across the cut", strings.Repeat("x", 2044) + string(spelled[1:len(spelled)-1]), strings.Repeat("x", 2044) + "<key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := startProvider(t, 401, tt.body)
			s := startWith(t, "", Options{Environ: []string{"TEST_KEY=" + escapedKey}})
			var res testResult
			if code := sendTestRequest(t, s, p.URL, "m", &res); code != 200 {
				t.Fatalf("POST /api/test: %d", code)
			}
			res.MS = 0
			if want := (testResult{Status: 401, Body: tt.want}); res != want {
				t.Errorf("result %+v, want %+v", res, want)
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

// TestRedact: each character of a key goes in every spelling that a JSON
// string, or a JSON string inside one, gives it; the rest of the text stays.
func TestRedact(t *testing.T) {
	type row struct{ name, s, key, want string }
	var tests []row
	for c := byte(0x21); c <= 0x7e; c++ {
		hex := fmt.Sprintf("%04x", c)
		spellings := []struct{ how, spelled string }{
			{"as it is", string(c)},
			{`as \u with lower-case hex`, `\u` + hex},
			{`as \u with upper-case hex`, `\u` + strings.ToUpper(hex)},
			{`as \\u, inside a nested string`, `\\u` + hex},
		}
		if c == '"' || c == '\\' || c == '/' {
			spellings = append(spellings,
				struct{ how, spelled string }{"behind a backslash", `\` + string(c)},
				struct{ how, spelled string }{"behind three backslashes, inside a nested string", `\\\` + string(c)})
		}
		for _, sp := range spellings {
			tests = append(tests, row{fmt.Sprintf("%q %s", c, sp.how), "a sk-" + sp.spelled + "z b", "sk-" + string(c) + "z", "a <key> b"})
		}
	}
	// A u or a backslash at the end of the key, written as an escape: the
	// escape goes whole, not only its start.
	tests = append(tests,
		row{"u at the end, as an escape", `a sk-\u0075 b`, "sk-u", "a <key> b"},
		row{"u at the end, as an escape inside a nested string", `a sk-\\u0075 b`, "sk-u", "a <key> b"},
		row{"u0 at the end, its u as an escape", `a sk-\u00750 b`, "sk-u0", "a <key> b"},
		row{"u007 at the end, its u as an escape", `a sk-\u0075007 b`, "sk-u007", "a <key> b"},
		row{"a backslash at the end, as an escape", `a sk-\u005c b`, "sk-\\", "a <key> b"},
		row{"a backslash at the end, as an escape inside a nested string", `a sk-\\u005c b`, "sk-\\", "a <key> b"},
		row{"a u at the end after a backslash, the u as an escape", `a sk-\\\u0075 b`, "sk-\\u", "a <key> b"},
	)
	// A level outside may write the backslash of an escape as \u005c, in
	// either case, and a level further out may write a backslash either way:
	// two decodes of the first two bodies give the key back, three or four of
	// the others.
	body := func(spelled string) string { return `{"error":{"message":"invalid key ` + spelled + `"}}` }
	tests = append(tests,
		row{`each \ as \u005c`, body(`\u005cu0073\u005cu006b\u005cu002d\u005cu007a`), "sk-z", body("<key>")},
		row{`each \ as \u005C`, body(`\u005Cu0073\u005Cu006b\u005Cu002d\u005Cu007a`), "sk-z", body("<key>")},
		row{`each \ as \u005c, three levels deep`, body(`\u005cu005cu0073\u005cu005cu006b\u005cu005cu002d\u005cu005cu007a`), "sk-z", body("<key>")},
		row{`each \ as \\ and \u005c, three levels deep`, body(`\\u005cu0073\\u005cu006b\\u005cu002d\\u005cu007a`), "sk-z", body("<key>")},
		row{`each \ as \\ and \u005c twice, four levels deep`, body(`\\u005cu005cu0073\\u005cu005cu006b\\u005cu005cu002d\\u005cu005cu007a`), "sk-z", body("<key>")},
	)
	tests = append(tests,
		row{"a text without the key keeps its escapes", `{"error":"no \"m\" at \u003c\/v1\u003e"}`, "sk-test", `{"error":"no \"m\" at \u003c\/v1\u003e"}`},
		row{"a text with only the start of the key", "a sk- b", "sk-test", "a sk- b"},
		row{"an empty key", "a sk- b", "", "a sk- b"},
	)
	for _, tt := range tests {
		if got := redact(tt.s, tt.key); got != tt.want {
			t.Errorf("%s: redact(%q, %q) = %q, want %q", tt.name, tt.s, tt.key, got, tt.want)
		}
	}
}

// TestRedactJSONEncoders: the key goes from an error body as the encoders of
// providers and gateways write it, and no decoding brings it back.
func TestRedactJSONEncoders(t *testing.T) {
	message := map[string]any{"error": map[string]any{"message": "invalid key " + escapedKey}}
	goBody, _ := json.Marshal(message) // with HTML escaping: \u003c for <
	var plain bytes.Buffer
	enc := json.NewEncoder(&plain)
	enc.SetEscapeHTML(false)
	enc.Encode(message)
	dotNet := strings.NewReplacer(`"`, `\u0022`, `\`, `\\`, `+`, `\u002B`, `'`, `\u0027`, `<`, `\u003C`, `>`, `\u003E`, `&`, `\u0026`).Replace(escapedKey)
	gateway, _ := json.Marshal(map[string]string{"error": "upstream: " + string(goBody)})
	tests := []struct{ name, body string }{
		{"Go", string(goBody)},
		{"Go without HTML escaping", plain.String()},
		{"PHP", strings.ReplaceAll(string(goBody), "/", `\/`)},
		{".NET", `{"error":{"message":"invalid key ` + dotNet + `"}}`},
		{"a gateway", string(gateway)},
	}
	for _, tt := range tests {
		got := redact(tt.body, escapedKey)
		if !strings.Contains(got, "<key>") {
			t.Errorf("%s: %s, want <key> in it", tt.name, got)
		}
		if found := keysIn(t, got, escapedKey); len(found) > 0 {
			t.Errorf("%s: %s decodes to %q", tt.name, got, found)
		}
	}
}

// keysIn returns the strings that hold key in body, a JSON text: its own
// strings and those of each string that holds JSON in turn, either as a whole
// or after a prefix, as the gateway's error holds the provider's.
func keysIn(t *testing.T, body, key string) []string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Errorf("%s: %v", body, err)
		return nil
	}
	var found []string
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, e := range v {
				walk(k)
				walk(e)
			}
		case []any:
			for _, e := range v {
				walk(e)
			}
		case string:
			if strings.Contains(v, key) {
				found = append(found, v)
			}
			var inner any
			if json.Unmarshal([]byte(v), &inner) == nil {
				walk(inner)
			} else if i := strings.IndexByte(v, '{'); i > 0 && json.Unmarshal([]byte(v[i:]), &inner) == nil {
				walk(inner)
			}
		}
	}
	walk(v)
	return found
}
