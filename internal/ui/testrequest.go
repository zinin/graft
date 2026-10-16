package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/edit"
	"github.com/zinin/graft/internal/router"
)

// testTimeout is how long a test request may take in all; a test makes it
// shorter.
var testTimeout = 60 * time.Second

const (
	maxAnswer    = 200     // characters of a 2xx answer that the page shows
	maxErrorBody = 2 << 10 // bytes of the body of another status
	maxRead      = 1 << 20 // bytes of a response the server reads
)

// testRequest is what Test sends: the values of the forms, which need not be
// saved yet.
type testRequest struct {
	URL string   `json:"url"`
	Key edit.Key `json:"key"`
	ID  string   `json:"id"`
}

// testResult is the provider's answer to a test request, without its key.
type testResult struct {
	Status int    `json:"status"`
	MS     int64  `json:"ms"`
	Text   string `json:"text"` // of a 2xx: the first text block; "" when there is none
	Body   string `json:"body"` // of another status
}

// testMessage is the body of a test request.
type testMessage struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []testContent `json:"messages"`
}

type testContent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (a *api) test(w http.ResponseWriter, r *http.Request) {
	var req testRequest
	if !readJSON(w, r, &req) {
		return
	}
	if msg := config.CheckURL(req.URL); msg != "" {
		a.fail(w, http.StatusUnprocessableEntity, errors.New("url "+msg))
		return
	}
	if req.ID == "" || strings.ContainsFunc(req.ID, unicode.IsSpace) {
		a.fail(w, http.StatusUnprocessableEntity, errors.New("the model needs an id or a name, without white space"))
		return
	}
	src, err := a.source(req.Key)
	if err != nil {
		a.fail(w, http.StatusUnprocessableEntity, fmt.Errorf("key: %w", err))
		return
	}
	// Without run: a key command may ask for a password, so only Run starts
	// it, and the test takes the key that Run kept.
	key, _, err := a.readKey(src, false)
	if err != nil {
		a.fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	res, err := sendTest(r.Context(), req.URL, key, req.ID)
	if err != nil {
		// Go's errors name the URL and not the headers, so the key should not
		// be there; redact makes sure, as for an answer.
		a.fail(w, http.StatusBadGateway, errors.New(redact(err.Error(), key)))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// sendTest asks model id at the provider at base for one word, as Claude
// Code would ask it: through the router's transport, with key as a bearer
// token, without a stream or retries, and without following a redirect,
// which the router does not follow either.
func sendTest(ctx context.Context, base, key, id string) (testResult, error) {
	u, err := url.Parse(base)
	if err != nil {
		return testResult{}, err
	}
	// One slash between the path of the URL and the API path, as the router
	// joins them: …/anthropic/ gets …/anthropic/v1/messages.
	u.Path, u.RawPath = strings.TrimSuffix(u.Path, "/")+"/v1/messages", ""
	body, _ := json.Marshal(testMessage{
		Model:     id,
		MaxTokens: 16,
		Messages:  []testContent{{Role: "user", Content: "Reply with one word: pong"}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return testResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Transport:     router.Transport(),
		Timeout:       testTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	// The transport is this request's own: no later request would take the
	// connection it keeps, so it is closed once the answer is read.
	defer client.CloseIdleConnections()
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return testResult{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRead))
	if err != nil {
		return testResult{}, err
	}
	res := testResult{Status: resp.StatusCode, MS: time.Since(start).Milliseconds()}
	// The key goes before the cut: a cut first could leave the start of a
	// key that the provider repeats.
	if resp.StatusCode/100 == 2 {
		res.Text = cutRunes(redact(firstText(data), key), maxAnswer)
	} else {
		res.Body = cutBytes(redact(string(data), key), maxErrorBody)
	}
	return res, nil
}

// firstText returns the text of the first text block of a Messages API
// answer, or "": a model that thinks may spend its 16 tokens before it
// writes.
func firstText(data []byte) string {
	var answer struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(data, &answer) != nil {
		return ""
	}
	for _, block := range answer.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}

// redact puts <key> in place of each occurrence of key in s.
func redact(s, key string) string {
	if key == "" {
		// ReplaceAll would put <key> between all the characters of s.
		return s
	}
	return strings.ReplaceAll(s, key, "<key>")
}

// cutRunes returns the first n characters of s.
func cutRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

// cutBytes returns at most the first n bytes of s, without a character cut
// in two.
func cutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
