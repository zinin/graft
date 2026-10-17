package ui

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/zinin/graft/internal/config"
)

// TestURLTableMatchesCheckURL checks jstest/urls.json, the table of URLs that
// draft.test.mjs checks urlError against: config.CheckURL gives each URL the
// message that the table gives it, an empty one for a URL it takes. So the
// dialog Add provider and the config cannot drift apart on these URLs.
func TestURLTableMatchesCheckURL(t *testing.T) {
	data, err := os.ReadFile("jstest/urls.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		URL   string `json:"url"`
		Error string `json:"error"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range cases {
		if got := config.CheckURL(c.URL); got != c.Error {
			t.Errorf("CheckURL(%q) = %q, want %q", c.URL, got, c.Error)
		}
	}
}
