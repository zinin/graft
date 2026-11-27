package config

import (
	"errors"
	"testing"
)

// TestEnvNamesThatDifferInCaseOnWindows: FOO and Foo are one variable on
// Windows, so one env may not give it two values.
func TestEnvNamesThatDifferInCaseOnWindows(t *testing.T) {
	_, err := parse(t, base+"profiles:\n  a: { model: m, env: { FOO: a, Foo: b } }\n")
	var errs Errors
	if !errors.As(err, &errs) || len(errs) != 1 || errs[0].Path != "profiles.a.env.Foo" || errs[0].Msg != "the same variable as FOO on Windows" {
		t.Errorf("error %v, want one at profiles.a.env.Foo: the same variable as FOO on Windows", err)
	}
}
