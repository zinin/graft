package router

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const secret = "0123456789abcdef0123456789abcdef"

// received is a request as a fake upstream got it.
type received struct {
	method, path, query, host string
	header                    http.Header
	length                    int64
	body                      string
}

// upstream is a fake provider or a fake Anthropic. It records each request
// and answers with handle, or with 200 and {"ok":true}.
type upstream struct {
	srv *httptest.Server
	mu  sync.Mutex
	got []received
}

// newUpstream starts a fake upstream. A fake provider fails the test when a
// header carries a credential of Anthropic, such as the claude.ai token.
func newUpstream(t *testing.T, provider bool, handle http.HandlerFunc) *upstream {
	t.Helper()
	u := &upstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		for name, values := range r.Header {
			for _, v := range values {
				if provider && strings.Contains(v, "sk-ant-") {
					t.Errorf("a provider got an Anthropic credential in %s", name)
				}
			}
		}
		u.mu.Lock()
		u.got = append(u.got, received{r.Method, r.URL.Path, r.URL.RawQuery, r.Host, r.Header.Clone(), r.ContentLength, string(body)})
		u.mu.Unlock()
		if handle != nil {
			handle(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) requests() []received {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.got)
}

// at returns the upstream's URL with path as the base path.
func (u *upstream) at(path string) *url.URL {
	base, _ := url.Parse(u.srv.URL + path)
	return base
}

func (u *upstream) host() string { return u.srv.Listener.Addr().String() }

// env is a router before two fake providers, cliproxy and ollama, whose API
// sits under /anthropic, and a fake Anthropic.
type env struct {
	url                         string // the router's
	cliproxy, ollama, anthropic *upstream
	log                         *lockedBuffer
}

// newEnv starts a router for profile mix, with the subscription, or multi,
// without it. The upstreams answer with handle.
func newEnv(t *testing.T, subscription bool, handle http.HandlerFunc) *env {
	t.Helper()
	e := &env{
		cliproxy:  newUpstream(t, true, handle),
		ollama:    newUpstream(t, true, handle),
		anthropic: newUpstream(t, false, handle),
		log:       &lockedBuffer{},
	}
	profile := "multi"
	if subscription {
		profile = "mix"
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// The server graft runs, with its timeouts.
	srv := NewServer(Config{
		Profile:      profile,
		Subscription: subscription,
		Routes: map[string]Route{
			"sol":     {Provider: "cliproxy", URL: e.cliproxy.at(""), Key: "sk-cliproxy", ID: "gpt-6.1-sol"},
			"glm":     {Provider: "ollama", URL: e.ollama.at("/anthropic"), Key: "sk-ollama", ID: "glm-5.3-flash"},
			"kimi-k3": {Provider: "ollama", URL: e.ollama.at("/anthropic/"), Key: "sk-ollama", ID: "kimi-k3"},
		},
		Secret:    secret,
		Anthropic: e.anthropic.at(""),
		Log:       NewLog(e.log, "abc123"),
	})
	go srv.Serve(ln)
	t.Cleanup(srv.Stop)
	e.url = "http://" + ln.Addr().String()
	return e
}

// waitForLine waits until the log has a line that contains want.
func waitForLine(t *testing.T, log *lockedBuffer, want string) string {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		for _, line := range strings.Split(log.String(), "\n") {
			if strings.Contains(line, want) {
				return line
			}
		}
	}
	t.Fatalf("no line with %q in the log:\n%s", want, log.String())
	return ""
}

func (e *env) upstreamRequests() int {
	return len(e.cliproxy.requests()) + len(e.ollama.requests()) + len(e.anthropic.requests())
}

// request returns a request to the router as Claude Code sends it with the
// claude.ai login and the session secret; edit changes its headers.
func (e *env) request(t *testing.T, method, path, body string, edit ...func(http.Header)) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, e.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	h := req.Header
	h.Set("Authorization", "Bearer sk-ant-oat01-login")
	h.Set("Anthropic-Beta", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14")
	h.Set("Anthropic-Version", "2023-06-01")
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", "claude-cli/2.1.296")
	h.Set("X-App", "cli")
	h.Set(SessionHeader, secret)
	for _, f := range edit {
		f(h)
	}
	return req
}

// answer is a response of the router, read whole.
type answer struct {
	status int
	header http.Header
	body   string
}

func read(t *testing.T, resp *http.Response) answer {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return answer{resp.StatusCode, resp.Header, string(body)}
}

// do sends a request made by request and reads the answer.
func (e *env) do(t *testing.T, method, path, body string, edit ...func(http.Header)) answer {
	t.Helper()
	resp, err := http.DefaultClient.Do(e.request(t, method, path, body, edit...))
	if err != nil {
		t.Fatal(err)
	}
	return read(t, resp)
}

// checkError checks that a is an error of the Anthropic API whose message
// starts with msg.
func checkError(t *testing.T, a answer, status int, typ, msg string) {
	t.Helper()
	var body struct {
		Type  string
		Error struct{ Type, Message string }
	}
	if a.status != status || json.Unmarshal([]byte(a.body), &body) != nil || body.Type != "error" ||
		body.Error.Type != typ || !strings.HasPrefix(body.Error.Message, msg) {
		t.Errorf("answer %d %s, want %d with %s %q", a.status, a.body, status, typ, msg)
	}
	if ct := a.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q", ct)
	}
}

func TestHello(t *testing.T) {
	e := newEnv(t, true, nil)
	a := e.do(t, http.MethodHead, "/api/hello", "", func(h http.Header) {
		h.Del(SessionHeader)
		h.Del("Authorization")
	})
	if a.status != http.StatusOK {
		t.Errorf("status %d, want 200 without the secret", a.status)
	}
	if n := e.upstreamRequests(); n != 0 {
		t.Errorf("%d requests went upstream, want graft to answer itself", n)
	}
}

func TestSessionSecret(t *testing.T) {
	tests := map[string]func(http.Header){
		"no secret":       func(h http.Header) { h.Del(SessionHeader) },
		"wrong secret":    func(h http.Header) { h.Set(SessionHeader, strings.Repeat("f", len(secret))) },
		"secret and more": func(h http.Header) { h.Set(SessionHeader, secret+"0") },
		"secret twice":    func(h http.Header) { h.Add(SessionHeader, secret) },
	}
	for name, edit := range tests {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, true, nil)
			for _, path := range []string{"/v1/messages", "/api/oauth/profile"} {
				a := e.do(t, http.MethodPost, path, `{"model":"sol"}`, edit)
				checkError(t, a, http.StatusForbidden, "permission_error",
					"graft: this router serves only its own Claude Code session")
			}
			if n := e.upstreamRequests(); n != 0 {
				t.Errorf("%d requests went upstream", n)
			}
		})
	}
}

func TestRouteByModel(t *testing.T) {
	e := newEnv(t, false, nil)
	e.do(t, http.MethodPost, "/v1/messages?beta=true", `{"model":"sol","max_tokens":5}`)
	e.do(t, http.MethodPost, "/v1/messages/count_tokens?beta=true", `{"model":"glm","messages":[]}`)
	e.do(t, http.MethodPost, "/v1/messages", `{"model":"kimi-k3"}`)

	cp := e.cliproxy.requests()
	if len(cp) != 1 || cp[0].method != http.MethodPost || cp[0].path != "/v1/messages" || cp[0].query != "beta=true" ||
		cp[0].body != `{"model":"gpt-6.1-sol","max_tokens":5}` {
		t.Errorf("cliproxy got %+v", cp)
	}
	ol := e.ollama.requests()
	if len(ol) != 2 ||
		ol[0].path != "/anthropic/v1/messages/count_tokens" || ol[0].query != "beta=true" ||
		ol[0].body != `{"model":"glm-5.3-flash","messages":[]}` ||
		ol[1].path != "/anthropic/v1/messages" || ol[1].body != `{"model":"kimi-k3"}` {
		t.Errorf("ollama got %+v", ol)
	}
	if n := len(e.anthropic.requests()); n != 0 {
		t.Errorf("anthropic got %d requests", n)
	}
}

func TestProviderHeaders(t *testing.T) {
	e := newEnv(t, true, nil)
	e.do(t, http.MethodPost, "/v1/messages", `{"model":"sol"}`, func(h http.Header) {
		h.Set("X-Api-Key", "sk-ant-api03-key")
	})
	got := e.cliproxy.requests()
	if len(got) != 1 {
		t.Fatalf("cliproxy got %d requests", len(got))
	}
	h := got[0].header
	for name, want := range map[string]string{
		"Authorization":     "Bearer sk-cliproxy",
		"X-Api-Key":         "",
		SessionHeader:       "",
		"Anthropic-Beta":    "claude-code-20250219,interleaved-thinking-2025-05-14",
		"Anthropic-Version": "2023-06-01",
		"User-Agent":        "claude-cli/2.1.296",
		"X-App":             "cli",
		"X-Forwarded-For":   "",
		"X-Forwarded-Host":  "",
		"X-Forwarded-Proto": "",
		"Forwarded":         "",
	} {
		if got := strings.Join(h.Values(name), " | "); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if got[0].host != e.cliproxy.host() {
		t.Errorf("Host = %q, want the provider's %q", got[0].host, e.cliproxy.host())
	}
}

func TestOnlyOAuthBetaGoes(t *testing.T) {
	e := newEnv(t, false, nil)
	e.do(t, http.MethodPost, "/v1/messages", `{"model":"sol"}`, func(h http.Header) {
		h.Set("Anthropic-Beta", "oauth-2025-04-20")
	})
	if got := e.cliproxy.requests()[0].header.Values("Anthropic-Beta"); got != nil {
		t.Errorf("anthropic-beta = %q, want no header", got)
	}
}

func TestBodyKeepsOtherBytes(t *testing.T) {
	e := newEnv(t, false, nil)
	in := "{\"max_tokens\":5, \"model\" : \"sol\" ,\n\t\"messages\":[{\"role\":\"user\",\"content\":\"say \\\"model\\\": sol\"}]}"
	want := "{\"max_tokens\":5, \"model\" : \"gpt-6.1-sol\" ,\n\t\"messages\":[{\"role\":\"user\",\"content\":\"say \\\"model\\\": sol\"}]}"
	e.do(t, http.MethodPost, "/v1/messages", in)
	got := e.cliproxy.requests()[0]
	if got.body != want {
		t.Errorf("body\n%s\nwant\n%s", got.body, want)
	}
	if got.length != int64(len(want)) {
		t.Errorf("Content-Length %d, want %d", got.length, len(want))
	}
}

func TestSubscriptionGoesToAnthropic(t *testing.T) {
	e := newEnv(t, true, nil)
	e.do(t, http.MethodPost, "/v1/messages?beta=true", `{"model":"claude-opus-5-5"}`)
	e.do(t, http.MethodPost, "/v1/messages", `not json`)
	e.do(t, http.MethodGet, "/api/oauth/profile", "")
	e.do(t, http.MethodPost, "/v1/messages", `{"model":"sol"}`)

	got := e.anthropic.requests()
	if len(got) != 3 {
		t.Fatalf("anthropic got %d requests, want 3", len(got))
	}
	first := got[0]
	if first.path != "/v1/messages" || first.query != "beta=true" || first.body != `{"model":"claude-opus-5-5"}` ||
		first.host != e.anthropic.host() ||
		first.header.Get("Authorization") != "Bearer sk-ant-oat01-login" ||
		first.header.Get("Anthropic-Beta") != "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14" ||
		first.header.Get(SessionHeader) != "" {
		t.Errorf("anthropic got %+v", first)
	}
	if got[1].body != "not json" || got[2].method != http.MethodGet || got[2].path != "/api/oauth/profile" {
		t.Errorf("anthropic got %+v and %+v", got[1], got[2])
	}
	if cp := e.cliproxy.requests(); len(cp) != 1 || cp[0].header.Get("Authorization") != "Bearer sk-cliproxy" {
		t.Errorf("cliproxy got %+v", cp)
	}
}

func TestWithoutSubscription(t *testing.T) {
	e := newEnv(t, false, nil)
	checkError(t, e.do(t, http.MethodPost, "/v1/messages", `{"model":"claude-opus-5-5"}`),
		http.StatusBadRequest, "invalid_request_error", `graft: model "claude-opus-5-5" is not configured in profile "multi"`)
	checkError(t, e.do(t, http.MethodPost, "/v1/messages/count_tokens", `not json`),
		http.StatusBadRequest, "invalid_request_error", `graft: profile "multi" has no route for a request without a model`)
	checkError(t, e.do(t, http.MethodPost, "/v1/messages/batches", `{"model":"sol"}`),
		http.StatusBadRequest, "invalid_request_error", `graft: profile "multi" has no route for POST /v1/messages/batches`)
	checkError(t, e.do(t, http.MethodGet, "/v1/models", ""),
		http.StatusNotFound, "not_found_error", `graft: profile "multi" has no route for GET /v1/models`)
	if n := e.upstreamRequests(); n != 0 {
		t.Errorf("%d requests went upstream", n)
	}
}

// TestStreamIsNotBuffered: the provider sends its second event only after
// the client has the first, so a router that held the stream back would
// never deliver both.
func TestStreamIsNotBuffered(t *testing.T) {
	first := make(chan struct{})
	e := newEnv(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: one\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-first:
			io.WriteString(w, "event: two\n\n")
		case <-time.After(5 * time.Second):
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(e.request(t, http.MethodPost, "/v1/messages", `{"model":"sol","stream":true}`).WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := bufio.NewReader(resp.Body)
	if line, err := body.ReadString('\n'); line != "event: one\n" {
		t.Fatalf("first line %q, %v", line, err)
	}
	close(first)
	if rest, _ := io.ReadAll(body); string(rest) != "\nevent: two\n\n" {
		t.Errorf("rest of the stream %q, want the second event", rest)
	}
}

func TestUpstreamErrorPassesUnchanged(t *testing.T) {
	const limited = `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`
	e := newEnv(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.Header().Set("X-Should-Retry", "true")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, limited)
	})
	a := e.do(t, http.MethodPost, "/v1/messages", `{"model":"sol"}`)
	if a.status != http.StatusTooManyRequests || a.header.Get("Retry-After") != "7" ||
		a.header.Get("X-Should-Retry") != "true" || a.body != limited {
		t.Errorf("answer %d %v %s", a.status, a.header, a.body)
	}
}

func TestUnreachableUpstream(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL, _ := url.Parse(dead.URL)
	dead.Close()
	tests := []struct {
		name, model, provider string
		subscription          bool
	}{
		{"provider", "sol", "cliproxy", false},
		{"anthropic", "claude-opus-5-5", "anthropic", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &lockedBuffer{}
			srv := httptest.NewServer(Handler(Config{
				Profile:      "p",
				Subscription: tt.subscription,
				Routes:       map[string]Route{"sol": {Provider: "cliproxy", URL: deadURL, Key: "sk-cliproxy", ID: "gpt-6.1-sol"}},
				Secret:       secret,
				Anthropic:    deadURL,
				Log:          NewLog(log, "abc123"),
			}))
			defer srv.Close()
			e := &env{url: srv.URL}
			checkError(t, e.do(t, http.MethodPost, "/v1/messages", `{"model":"`+tt.model+`"}`),
				http.StatusBadGateway, "api_error", "graft: provider "+tt.provider+" unreachable: ")
			if line := waitForLine(t, log, "route="+tt.provider+" status=502 "); !strings.Contains(line, ` err="dial tcp `) {
				t.Errorf("log line %q, want the error", line)
			}
		})
	}
}

// TestTooLarge sends only the headers of a request: the router must refuse
// it by its Content-Length, before the body.
func TestTooLarge(t *testing.T) {
	e := newEnv(t, false, nil)
	conn, err := net.Dial("tcp", strings.TrimPrefix(e.url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second)) // a router waiting for the body never answers
	fmt.Fprintf(conn, "POST /v1/messages HTTP/1.1\r\nHost: router\r\n%s: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n",
		SessionHeader, secret, 64<<20+1)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	checkError(t, read(t, resp), http.StatusRequestEntityTooLarge, "request_too_large", "graft: the request body is larger than 64 MiB")
	if n := e.upstreamRequests(); n != 0 {
		t.Errorf("%d requests went upstream", n)
	}
}

func TestClientGoneCancelsUpstream(t *testing.T) {
	canceled := make(chan struct{})
	e := newEnv(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(5 * time.Second):
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	resp, err := http.DefaultClient.Do(e.request(t, http.MethodPost, "/v1/messages", `{"model":"sol","stream":true}`).WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	resp.Body.Close()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the request to the provider went on after the client had gone")
	}
	if line := waitForLine(t, e.log, "model=sol route=cliproxy status=200 "); !strings.HasSuffix(line, " canceled=true") {
		t.Errorf("log line %q, want canceled=true", line)
	}
}

func TestClientGoneBeforeTheAnswer(t *testing.T) {
	arrived := make(chan struct{})
	e := newEnv(t, false, func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-arrived
		cancel()
	}()
	if _, err := http.DefaultClient.Do(e.request(t, http.MethodPost, "/v1/messages", `{"model":"sol"}`).WithContext(ctx)); err == nil {
		t.Fatal("the request did not fail")
	}
	line := waitForLine(t, e.log, "model=sol route=cliproxy ")
	if !strings.Contains(line, " status=0 ") || !strings.HasSuffix(line, " canceled=true") {
		t.Errorf("log line %q, want no status and canceled=true", line)
	}
}

func TestRequestsAreLogged(t *testing.T) {
	e := newEnv(t, true, nil)
	e.do(t, http.MethodHead, "/api/hello", "", func(h http.Header) { h.Del(SessionHeader) })
	e.do(t, http.MethodPost, "/v1/messages", `{"model":"sol"}`, func(h http.Header) { h.Del(SessionHeader) })
	e.do(t, http.MethodPost, "/v1/messages?beta=true", `{"model":"sol","messages":[{"role":"user","content":"private words"}]}`)
	e.do(t, http.MethodPost, "/v1/messages/count_tokens", `{"model":"claude-opus-5-5"}`)
	e.do(t, http.MethodGet, "/api/oauth/profile", "")
	dur := regexp.MustCompile(` dur=\d+\.\ds`)
	var got []string
	for _, line := range lines(t, e.log.String()) {
		got = append(got, dur.ReplaceAllString(line, ""))
	}
	want := []string{
		"session=abc123 HEAD /api/hello route=graft status=200 bytes=0",
		fmt.Sprintf("session=abc123 POST /v1/messages route=graft status=403 bytes=%d",
			len(`{"type":"error","error":{"type":"permission_error","message":"graft: this router serves only its own Claude Code session"}}`)),
		"session=abc123 POST /v1/messages model=sol route=cliproxy status=200 bytes=11",
		"session=abc123 POST /v1/messages/count_tokens model=claude-opus-5-5 route=anthropic status=200 bytes=11",
		"session=abc123 GET /api/oauth/profile route=anthropic status=200 bytes=11",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("log:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, private := range []string{secret, "sk-cliproxy", "sk-ollama", "sk-ant-oat01", "private words", "beta=true"} {
		if strings.Contains(e.log.String(), private) {
			t.Errorf("the log shows %q", private)
		}
	}
}

// TestSlowAnswer: a model may think for minutes before its first byte, so
// no timeout of graft may cut a slow answer off.
func TestSlowAnswer(t *testing.T) {
	t.Parallel()
	e := newEnv(t, false, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(11 * time.Second) // longer than every timeout graft has
		io.WriteString(w, `{"ok":true}`)
	})
	if a := e.do(t, http.MethodPost, "/v1/messages", `{"model":"sol"}`); a.status != http.StatusOK || a.body != `{"ok":true}` {
		t.Errorf("answer %d %s", a.status, a.body)
	}
}

// slowToNotice is a provider that takes a while to see that a request was
// canceled.
type slowToNotice struct{ arrived chan struct{} }

func (p slowToNotice) RoundTrip(r *http.Request) (*http.Response, error) {
	close(p.arrived)
	<-r.Context().Done()
	time.Sleep(300 * time.Millisecond)
	return nil, r.Context().Err()
}

// TestStopKeepsTheLinesOfRequestsItEnds: when claude exits, its requests
// under way end, and their lines must reach the log before the line that
// ends the session.
func TestStopKeepsTheLinesOfRequestsItEnds(t *testing.T) {
	log := &lockedBuffer{}
	provider := slowToNotice{arrived: make(chan struct{})}
	srv := NewServer(Config{
		Profile:   "multi",
		Routes:    map[string]Route{"sol": {Provider: "cliproxy", URL: &url.URL{Scheme: "http", Host: "provider.invalid"}, Key: "sk-cliproxy", ID: "gpt-6.1-sol"}},
		Secret:    secret,
		Transport: provider,
		Log:       NewLog(log, "abc123"),
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	e := &env{url: "http://" + ln.Addr().String()}
	ctx, cancel := context.WithCancel(context.Background())
	go http.DefaultClient.Do(e.request(t, http.MethodPost, "/v1/messages", `{"model":"sol"}`).WithContext(ctx))
	select {
	case <-provider.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("the request did not reach the provider")
	}
	cancel() // claude exits
	srv.Stop()
	if !strings.Contains(log.String(), " POST /v1/messages model=sol route=cliproxy status=0 ") {
		t.Errorf("Stop returned before the line of the request:\n%s", log.String())
	}
}

// TestServeLogsWhyItEnded: if the router stops serving before Stop, claude's
// requests fail, and only the log can say why. Stop is no failure.
func TestServeLogsWhyItEnded(t *testing.T) {
	tests := []struct {
		name string
		end  func(*Server, net.Listener)
		want *regexp.Regexp // the log; nil for none
	}{
		{"socket gone", func(_ *Server, ln net.Listener) { ln.Close() }, regexp.MustCompile(`^\S+ session=abc123 serve err="accept tcp [^"]+"\n$`)},
		{"Stop", func(s *Server, _ net.Listener) { s.Stop() }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &lockedBuffer{}
			srv := NewServer(Config{Secret: secret, Log: NewLog(log, "abc123")})
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				srv.Serve(ln)
				close(done)
			}()
			tt.end(srv, ln)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("Serve did not return")
			}
			if got := log.String(); tt.want == nil && got != "" || tt.want != nil && !tt.want.MatchString(got) {
				t.Errorf("log:\n%s", got)
			}
		})
	}
}
