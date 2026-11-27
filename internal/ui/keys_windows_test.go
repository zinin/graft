package ui

import (
	"testing"

	"github.com/zinin/graft/internal/keys"
)

// TestKeyCheckEnvInAnyCaseOnWindows: graft finds env OLLAMA_API_KEY in the
// variable Ollama_Api_Key on Windows, so the page must too.
func TestKeyCheckEnvInAnyCaseOnWindows(t *testing.T) {
	s := startWith(t, "", Options{Environ: []string{"Ollama_Api_Key=sk-env"}})
	if got, want := s.checkKey(t, "env", "OLLAMA_API_KEY", false), (keyState{State: "ok", Fingerprint: keys.Fingerprint("sk-env")}); got != want {
		t.Errorf("env OLLAMA_API_KEY: %+v, want %+v", got, want)
	}
}
