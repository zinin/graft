package launch

import (
	"slices"
	"testing"

	"github.com/zinin/graft/internal/plan"
)

// TestEnvironDropsAnyCaseOnWindows: Windows and Claude Code read a variable
// in any case, so graft removes a key from the shell however it is written.
func TestEnvironDropsAnyCaseOnWindows(t *testing.T) {
	p := &plan.Plan{Unset: []string{"ANTHROPIC_API_KEY"}, Set: []plan.EnvVar{{Name: "ANTHROPIC_BASE_URL", Value: "http://new"}}}
	got, err := Environ([]string{`Path=C:\bin`, "anthropic_api_key=sk-leak", "Anthropic_Base_Url=http://old"}, p, nil, nil)
	want := []string{`Path=C:\bin`, "ANTHROPIC_BASE_URL=http://new"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("Environ = %q, %v; want %q", got, err, want)
	}
}
