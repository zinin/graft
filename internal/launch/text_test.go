package launch

import (
	"errors"
	"testing"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/plan"
)

// shownKey stands in for the key of a provider in the tests of PlanText.
func shownKey(provider string) (string, error) { return "<key of " + provider + ">", nil }

func TestPlanTextDirect(t *testing.T) {
	p := &plan.Plan{
		Profile:   "glm",
		Mode:      config.Direct,
		Providers: []string{"ollama"},
		Args:      []string{"--model", "glm-5.3-flash", "--settings", `{"env":{}}`, "--resume"},
		Unset:     []string{"ANTHROPIC_API_KEY"},
		Set: []plan.EnvVar{
			{Name: "ANTHROPIC_BASE_URL", Value: "https://ollama.com"},
			{Name: "ANTHROPIC_AUTH_TOKEN", KeyOf: "ollama"},
		},
		Settings: map[string]any{"env": map[string]any{}},
	}
	got, err := PlanText(p, "/opt/claude/bin/claude", shownKey)
	if err != nil {
		t.Fatal(err)
	}
	want := `profile glm: direct, provider ollama
unset  ANTHROPIC_API_KEY
set    ANTHROPIC_BASE_URL=https://ollama.com
       ANTHROPIC_AUTH_TOKEN=<key of ollama>
exec   /opt/claude/bin/claude --model glm-5.3-flash --settings '{"env":{}}' --resume
settings
{
  "env": {}
}
`
	if got != want {
		t.Errorf("PlanText:\n%s\nwant:\n%s", got, want)
	}
}

func TestPlanTextRouter(t *testing.T) {
	p := &plan.Plan{
		Profile:      "mix",
		Mode:         config.Router,
		Subscription: true,
		Providers:    []string{"ollama", "cliproxy"},
		Routes: []plan.Route{
			{Model: "glm", Provider: "ollama", URL: "https://ollama.com", ID: "glm-5.3-flash"},
			{Model: "sol", Provider: "cliproxy", URL: "http://127.0.0.1:8317", ID: "gpt-6.1-sol"},
		},
		Args: []string{"--model", "opus", "--settings", "{}"},
		Set: []plan.EnvVar{
			{Name: "ANTHROPIC_BASE_URL", Session: plan.SessionURL},
			{Name: "ANTHROPIC_CUSTOM_HEADERS", Session: plan.SessionHeader},
		},
		Settings: map[string]any{"a": "<b>"},
	}
	got, err := PlanText(p, "/opt/claude/bin/claude", shownKey)
	if err != nil {
		t.Fatal(err)
	}
	want := `profile mix: router, providers ollama, cliproxy, subscription
set    ANTHROPIC_BASE_URL=http://127.0.0.1:<port>
       ANTHROPIC_CUSTOM_HEADERS=X-Graft-Session: <session secret>
route  glm -> ollama glm-5.3-flash <key of ollama>
       sol -> cliproxy gpt-6.1-sol <key of cliproxy>
exec   /opt/claude/bin/claude --model opus --settings '{}'
settings
{
  "a": "<b>"
}
`
	if got != want {
		t.Errorf("PlanText:\n%s\nwant:\n%s", got, want)
	}
}

func TestPlanTextStopsAtAKeyItCannotShow(t *testing.T) {
	p := &plan.Plan{Profile: "glm", Set: []plan.EnvVar{{Name: "ANTHROPIC_AUTH_TOKEN", KeyOf: "ollama"}}}
	refused := errors.New("provider ollama: env OLLAMA_API_KEY is not set")
	_, err := PlanText(p, "claude", func(string) (string, error) { return "", refused })
	if !errors.Is(err, refused) {
		t.Errorf("error %v, want %v", err, refused)
	}
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"/bin/claude", "--model", "a b", "it's", "", `{"x":1}`})
	want := `/bin/claude --model 'a b' 'it'\''s' '' '{"x":1}'`
	if got != want {
		t.Errorf("shellJoin = %s, want %s", got, want)
	}
}
