package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

// findModel finds the model of a request to the Messages API: the string
// "model" at the top level of a JSON object. It returns the model and where
// its value lies in body. When "model" repeats, the last one counts, as with
// JSON parsers. ok is false when body is not one JSON object or its model is
// not a string.
func findModel(body []byte) (model string, start, end int, ok bool) {
	if !json.Valid(body) {
		return "", 0, 0, false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return "", 0, 0, false
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return "", 0, 0, false
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return "", 0, 0, false
		}
		if key != "model" {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			model, ok = "", false
			continue
		}
		// The decoder stops right after the value, which raw holds whole.
		end = int(dec.InputOffset())
		model, start, ok = s, end-len(raw), true
	}
	return model, start, end, ok
}

// withModel returns a copy of body with the value at [start, end) replaced
// by id as a JSON string. The other bytes stay as they were.
func withModel(body []byte, start, end int, id string) []byte {
	quoted, _ := json.Marshal(id)
	out := make([]byte, 0, len(body)-(end-start)+len(quoted))
	out = append(out, body[:start]...)
	out = append(out, quoted...)
	return append(out, body[end:]...)
}

// dropOAuthBetas removes from anthropic-beta the oauth-… values, which come
// with the claude.ai login and mean nothing to a provider, and the header
// when nothing else is left. A header without them stays as it was.
func dropOAuthBetas(h http.Header) {
	var keep []string
	dropped := false
	for _, line := range h.Values("Anthropic-Beta") {
		for _, v := range strings.Split(line, ",") {
			switch v = strings.TrimSpace(v); {
			case strings.HasPrefix(v, "oauth-"):
				dropped = true
			case v != "":
				keep = append(keep, v)
			}
		}
	}
	switch {
	case !dropped:
	case len(keep) == 0:
		h.Del("Anthropic-Beta")
	default:
		h.Set("Anthropic-Beta", strings.Join(keep, ","))
	}
}
