// Package launch starts Claude Code from a plan: in place of graft in the
// direct mode, as graft's child behind the router otherwise.
package launch

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/zinin/graft/internal/osenv"
	"github.com/zinin/graft/internal/plan"
	"github.com/zinin/graft/internal/router"
)

// KeyFunc returns the key of a provider.
type KeyFunc func(provider string) (string, error)

// Session is what the running router puts into claude's environment.
type Session struct {
	Port   int
	Secret string
}

// SessionValue returns the value of a session variable for the router's
// port and secret; --dry-run passes placeholders for them.
func SessionValue(v plan.Session, port, secret string) string {
	if v == plan.SessionURL {
		return "http://127.0.0.1:" + port
	}
	return router.SessionHeader + ": " + secret
}

// Environ returns the environment for claude: environ without the variables
// the plan removes or sets, in any case on Windows, where Claude Code reads
// them so, then the plan's variables in order, with the provider keys read
// through key and the values of the router's session s.
func Environ(environ []string, p *plan.Plan, key KeyFunc, s *Session) ([]string, error) {
	drop := slices.Clone(p.Unset)
	for _, e := range p.Set {
		drop = append(drop, e.Name)
	}
	out := make([]string, 0, len(environ)+len(p.Set))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.ContainsFunc(drop, func(d string) bool { return osenv.Equal(d, name) }) {
			out = append(out, kv)
		}
	}
	for _, e := range p.Set {
		val := e.Value
		switch {
		case e.KeyOf != "":
			k, err := key(e.KeyOf)
			if err != nil {
				return nil, err
			}
			val = k
		case e.Session != 0 && s == nil:
			return nil, fmt.Errorf("%s needs the router", e.Name)
		case e.Session != 0:
			val = SessionValue(e.Session, strconv.Itoa(s.Port), s.Secret)
		}
		out = append(out, e.Name+"="+val)
	}
	return out, nil
}

// Exec replaces the current process with claude from PATH (see FindClaude),
// started as the plan says. lookPath and exec are exec.LookPath and Replace
// outside tests. Exec returns only if the start fails.
func Exec(p *plan.Plan, environ []string, key KeyFunc,
	lookPath func(string) (string, error), exec func(argv0 string, argv, envv []string) error) error {
	path, err := FindClaude(lookPath)
	if err != nil {
		return err
	}
	env, err := Environ(environ, p, key, nil)
	if err != nil {
		return err
	}
	if err := exec(path, append([]string{"claude"}, p.Args...), env); err != nil {
		return fmt.Errorf("start %s: %w", path, err)
	}
	return nil
}
