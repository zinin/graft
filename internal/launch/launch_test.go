package launch

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/plan"
)

var testPlan = &plan.Plan{
	Args:  []string{"--model", "m", "--resume"},
	Unset: []string{"ANTHROPIC_API_KEY"},
	Set: []plan.EnvVar{
		{Name: "ANTHROPIC_BASE_URL", Value: "https://provider.example"},
		{Name: "ANTHROPIC_AUTH_TOKEN", KeyOf: "p"},
		{Name: "FOO", Value: "new"},
	},
}

func keyOf(provider string) (string, error) {
	if provider != "p" {
		return "", errors.New("unexpected provider " + provider)
	}
	return "sk-provider", nil
}

func TestEnviron(t *testing.T) {
	got, err := Environ([]string{
		"PATH=/usr/bin", "ANTHROPIC_API_KEY=sk-ant-leak", "FOO=old", "ANTHROPIC_BASE_URL=http://old", "HOME=/home/u",
	}, testPlan, keyOf, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PATH=/usr/bin", "HOME=/home/u",
		"ANTHROPIC_BASE_URL=https://provider.example", "ANTHROPIC_AUTH_TOKEN=sk-provider", "FOO=new",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Environ =\n%q\nwant\n%q", got, want)
	}
}

func TestEnvironDropsEveryCopy(t *testing.T) {
	got, err := Environ([]string{"FOO=1", "ANTHROPIC_API_KEY=", "FOO=2", "ANTHROPIC_API_KEY=sk-ant-leak"}, testPlan, keyOf, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	if strings.Count(joined, "FOO=") != 1 || !strings.Contains(joined, "FOO=new") || strings.Contains(joined, "ANTHROPIC_API_KEY") {
		t.Errorf("Environ = %q, want one FOO=new and no ANTHROPIC_API_KEY", got)
	}
}

func TestEnvironKeyError(t *testing.T) {
	_, err := Environ(nil, testPlan, func(string) (string, error) { return "", errors.New("env K is not set") }, nil)
	if err == nil || err.Error() != "env K is not set" {
		t.Fatalf("error = %v", err)
	}
}

func TestEnvironSession(t *testing.T) {
	p := &plan.Plan{Set: []plan.EnvVar{
		{Name: "ANTHROPIC_BASE_URL", Session: plan.SessionURL},
		{Name: "ANTHROPIC_CUSTOM_HEADERS", Session: plan.SessionHeader},
	}}
	got, err := Environ(nil, p, keyOf, &Session{Port: 43117, Secret: "0a1b2c"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ANTHROPIC_BASE_URL=http://127.0.0.1:43117", "ANTHROPIC_CUSTOM_HEADERS=X-Graft-Session: 0a1b2c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Environ = %q, want %q", got, want)
	}
	if _, err := Environ(nil, p, keyOf, nil); err == nil || err.Error() != "ANTHROPIC_BASE_URL needs the router" {
		t.Errorf("without a session: error %v", err)
	}
}

func TestExec(t *testing.T) {
	var gotPath string
	var gotArgv, gotEnv []string
	err := Exec(testPlan, []string{"PATH=/usr/bin"}, keyOf,
		func(name string) (string, error) { return "/opt/bin/" + name, nil },
		func(path string, argv, env []string) error {
			gotPath, gotArgv, gotEnv = path, argv, env
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/opt/bin/claude" {
		t.Errorf("path = %q", gotPath)
	}
	if want := []string{"claude", "--model", "m", "--resume"}; !reflect.DeepEqual(gotArgv, want) {
		t.Errorf("argv = %q, want %q", gotArgv, want)
	}
	if !strings.Contains(strings.Join(gotEnv, "\n"), "ANTHROPIC_AUTH_TOKEN=sk-provider") {
		t.Errorf("env = %q, want the key", gotEnv)
	}
}

func TestExecWithoutClaude(t *testing.T) {
	err := Exec(testPlan, nil, keyOf,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, []string, []string) error { t.Fatal("exec called"); return nil })
	if err == nil || err.Error() != "claude not found in PATH" {
		t.Fatalf("error = %v", err)
	}
}

func TestExecFailure(t *testing.T) {
	err := Exec(testPlan, nil, keyOf,
		func(string) (string, error) { return "/opt/bin/claude", nil },
		func(string, []string, []string) error { return errors.New("permission denied") })
	if err == nil || err.Error() != "start /opt/bin/claude: permission denied" {
		t.Fatalf("error = %v", err)
	}
}
