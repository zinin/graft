// Package router is graft's local router. It serves one Claude Code session:
// a request for an external model goes to the model's provider, and with the
// subscription every other request goes to Anthropic.
package router

import (
	"bytes"
	"cmp"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

// AnthropicURL is where the requests of Claude models go. It lives in the
// code and not in the config, so that no typo can send the claude.ai token
// to another server.
const AnthropicURL = "https://api.anthropic.com"

// SessionHeader carries the session secret. Claude Code sends it, from
// ANTHROPIC_CUSTOM_HEADERS, with every request but HEAD /api/hello.
const SessionHeader = "X-Graft-Session"

// maxBody is the largest request body the router reads to find the model.
const maxBody = 64 << 20

// Route is where the requests of one external model go.
type Route struct {
	Provider string   // the provider's name, for messages
	URL      *url.URL // the provider's base URL
	Key      string   // the provider's key
	ID       string   // the model ID the provider expects
}

// Config is what a router serves.
type Config struct {
	Profile      string           // for messages
	Subscription bool             // whether the requests no route takes go to Anthropic
	Routes       map[string]Route // by the graft model name, which Claude Code sends
	Secret       string           // the session secret
	Anthropic    *url.URL         // nil means AnthropicURL; tests put a fake here
	Transport    http.RoundTripper
	Log          *Log
}

// Server is the router of a session on its socket.
type Server struct {
	srv *http.Server
	log *Log
}

// NewServer returns the router's server: 10 seconds for the headers of a
// request and no other limit, since a response may stream for minutes.
func NewServer(cfg Config) *Server {
	return &Server{
		srv: &http.Server{
			Handler:           Handler(cfg),
			ReadHeaderTimeout: 10 * time.Second,
			ErrorLog:          cfg.Log.logger(),
		},
		log: cfg.Log,
	}
}

// Serve serves the session on ln until Stop. If it fails before that, the
// log says why: claude's requests fail then, with no word from graft.
func (s *Server) Serve(ln net.Listener) {
	if err := s.srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		s.log.Printf("serve err=%q", err)
	}
}

// Stop closes the socket and ends the requests under way. Those whose client
// has gone, as claude's when it exits, end at once; the others get a second
// before they are canceled. The lines of the requests that end so reach the
// log before Stop returns.
func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.srv.Shutdown(ctx)
	s.srv.Close()
}

// Handler returns the router. A nil Transport means one with graft's
// timeouts and the proxy of the environment.
func Handler(cfg Config) http.Handler {
	if cfg.Anthropic == nil {
		cfg.Anthropic, _ = url.Parse(AnthropicURL)
	}
	if cfg.Transport == nil {
		cfg.Transport = transport()
	}
	return &router{cfg: cfg}
}

type router struct {
	cfg Config
}

// target is where the router sends a request.
type target struct {
	name string   // the provider's name, or anthropic
	url  *url.URL // its base URL
	key  string   // the provider's key; empty for Anthropic, which gets the request's own credentials
}

// ServeHTTP serves a request and writes its log line once it is done.
func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := &recorder{ResponseWriter: w}
	q := &request{method: r.Method, path: r.URL.Path, route: "graft"}
	start := time.Now()
	defer func() {
		q.status, q.bytes, q.dur = rec.status, rec.bytes, time.Since(start)
		v := recover()
		switch {
		case v == nil:
		case v != http.ErrAbortHandler:
			q.err = fmt.Sprintf("panic: %v", v)
		case r.Context().Err() != nil || rec.failed:
			q.canceled = true
		default:
			// ReverseProxy aborts the response when the stream breaks.
			q.err = "the response broke off"
		}
		rt.cfg.Log.request(q)
		if v != nil {
			panic(v)
		}
	}()
	rt.serve(rec, r, q)
}

func (rt *router) serve(w http.ResponseWriter, r *http.Request, q *request) {
	switch {
	case r.Method == http.MethodHead && r.URL.Path == "/api/hello":
		// Claude Code checks the address with it, and sends no secret.
		w.WriteHeader(http.StatusOK)
	case !rt.authorized(r):
		writeError(w, http.StatusForbidden, "permission_error", "graft: this router serves only its own Claude Code session")
	case r.Method == http.MethodPost && (r.URL.Path == "/v1/messages" || r.URL.Path == "/v1/messages/count_tokens"):
		rt.messages(w, r, q)
	case rt.cfg.Subscription:
		rt.forward(w, r, q, rt.anthropic(), nil)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/messages/"):
		writeError(w, http.StatusBadRequest, "invalid_request_error", rt.noRoute(r.Method+" "+r.URL.Path))
	default:
		writeError(w, http.StatusNotFound, "not_found_error", rt.noRoute(r.Method+" "+r.URL.Path))
	}
}

// authorized reports whether r carries the session secret, compared in
// constant time.
func (rt *router) authorized(r *http.Request) bool {
	got := r.Header.Values(SessionHeader)
	return len(got) == 1 && subtle.ConstantTimeCompare([]byte(got[0]), []byte(rt.cfg.Secret)) == 1
}

// messages sends a request of the Messages API where its model goes.
func (rt *router) messages(w http.ResponseWriter, r *http.Request, q *request) {
	if r.ContentLength > maxBody {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "graft: the request body is larger than 64 MiB")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "graft: the request body is larger than 64 MiB")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request_error", "graft: cannot read the request: "+err.Error())
		return
	}
	model, start, end, ok := findModel(body)
	q.model = model
	route, external := rt.cfg.Routes[model]
	switch {
	case ok && external:
		if route.ID != model {
			body = withModel(body, start, end, route.ID)
		}
		rt.forward(w, r, q, target{name: route.Provider, url: route.URL, key: route.Key}, body)
	case rt.cfg.Subscription:
		rt.forward(w, r, q, rt.anthropic(), body)
	case ok:
		writeError(w, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("graft: model %q is not configured in profile %q", model, rt.cfg.Profile))
	default:
		writeError(w, http.StatusBadRequest, "invalid_request_error", rt.noRoute("a request without a model"))
	}
}

// forward sends r to t, with body in place of the request's own when body is
// not nil, and passes the response back piece by piece as it comes.
func (rt *router) forward(w http.ResponseWriter, r *http.Request, q *request, t target, body []byte) {
	q.route = t.name
	if body != nil {
		r = r.Clone(r.Context())
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		// Lets the transport send the request again on a fresh connection
		// when a kept-alive one turns out closed before it took any of it.
		r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(t.url)
			h := pr.Out.Header
			h.Del(SessionHeader)
			if t.key != "" {
				// A provider gets its own key and nothing of the claude.ai login.
				h.Del("X-Api-Key")
				h.Set("Authorization", "Bearer "+t.key)
				dropOAuthBetas(h)
			}
		},
		Transport:     rt.cfg.Transport,
		FlushInterval: -1,
		ErrorLog:      rt.cfg.Log.logger(),
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			if req.Context().Err() != nil {
				q.canceled = true // the client has gone
				return
			}
			q.err = err.Error()
			writeError(w, http.StatusBadGateway, "api_error", fmt.Sprintf("graft: provider %s unreachable: %v", t.name, err))
		},
	}
	proxy.ServeHTTP(w, r)
}

func (rt *router) anthropic() target {
	return target{name: "anthropic", url: rt.cfg.Anthropic}
}

func (rt *router) noRoute(what string) string {
	return fmt.Sprintf("graft: profile %q has no route for %s", rt.cfg.Profile, what)
}

// writeError answers with an error in the format of the Anthropic API, which
// Claude Code shows to the user.
func writeError(w http.ResponseWriter, status int, typ, msg string) {
	var body struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	body.Type, body.Error.Type, body.Error.Message = "error", typ, msg
	data, _ := json.Marshal(body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

// transport returns the transport to providers and Anthropic: through the
// proxy of the environment, as Claude Code would go; 10 seconds to connect
// and 10 for the TLS handshake; no limit on the response, which may take
// minutes of thinking. It adds no compression of its own, so a response
// passes as the provider sent it.
func transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = proxy
	t.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.TLSHandshakeTimeout = 10 * time.Second
	t.DisableCompression = true
	return t
}

// proxy returns the proxy of a request as Claude Code picks it: without
// HTTPS_PROXY, https goes through HTTP_PROXY, where Go would go direct.
// NO_PROXY and the addresses of this machine work as in Go.
func proxy(req *http.Request) (*url.URL, error) {
	if req.URL.Scheme == "https" && os.Getenv("HTTPS_PROXY") == "" && os.Getenv("https_proxy") == "" {
		// Go takes HTTP_PROXY for the same host and port over http.
		u := *req.URL
		u.Scheme = "http"
		u.Host = net.JoinHostPort(u.Hostname(), cmp.Or(u.Port(), "443"))
		req = &http.Request{URL: &u}
	}
	return http.ProxyFromEnvironment(req)
}
