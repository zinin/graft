// Package launch starts Claude Code from a plan.
package launch

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zinin/graft/internal/plan"
)

// KeyFunc returns the key of a provider.
type KeyFunc func(provider string) (string, error)

// Environ returns the environment for claude: environ without the variables
// the plan removes or sets, then the plan's variables in order, with the
// provider keys read through key.
func Environ(environ []string, p *plan.Plan, key KeyFunc) ([]string, error) {
	drop := map[string]bool{}
	for _, name := range p.Unset {
		drop[name] = true
	}
	for _, e := range p.Set {
		drop[e.Name] = true
	}
	out := make([]string, 0, len(environ)+len(p.Set))
	for _, kv := range environ {
		if name, _, _ := strings.Cut(kv, "="); !drop[name] {
			out = append(out, kv)
		}
	}
	for _, e := range p.Set {
		val := e.Value
		if e.KeyOf != "" {
			k, err := key(e.KeyOf)
			if err != nil {
				return nil, err
			}
			val = k
		}
		out = append(out, e.Name+"="+val)
	}
	return out, nil
}

// Exec replaces the current process with claude from PATH, started as the
// plan says. lookPath and exec are exec.LookPath and syscall.Exec outside
// tests. Exec returns only if the start fails.
func Exec(p *plan.Plan, environ []string, key KeyFunc,
	lookPath func(string) (string, error), exec func(argv0 string, argv, envv []string) error) error {
	path, err := lookPath("claude")
	if err != nil {
		return errors.New("claude not found in PATH")
	}
	env, err := Environ(environ, p, key)
	if err != nil {
		return err
	}
	if err := exec(path, append([]string{"claude"}, p.Args...), env); err != nil {
		return fmt.Errorf("start %s: %w", path, err)
	}
	return nil
}
