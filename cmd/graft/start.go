package main

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/keys"
	"github.com/zinin/graft/internal/launch"
	"github.com/zinin/graft/internal/plan"
)

// start launches claude for profile, or prints the plan with --dry-run.
func start(cfg *config.Config, profile string, args []string, dryRun bool, sys system) int {
	userDir, err := plan.UserSettingsDir(cfg, profile, sys.environ, sys.home)
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	files, warnings := plan.LoadClaudeSettings(userDir, sys.cwd)
	for _, w := range warnings {
		fmt.Fprintf(sys.stderr, "graft: warning: %s\n", w)
	}
	p, err := plan.Build(plan.Input{
		Config:         cfg,
		Profile:        profile,
		Args:           args,
		Environ:        sys.environ,
		ClaudeSettings: files,
	})
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	if p.Mode == config.Router {
		fmt.Fprintf(sys.stderr, "graft: profile %q needs the router, which is not implemented yet\n", p.Profile)
		return 1
	}
	key := keyFunc(cfg, sys)
	if dryRun {
		return printPlan(cfg, p, key, sys)
	}
	if err := launch.Exec(p, sys.environ, key, sys.lookPath, sys.exec); err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	return 0 // only in tests: a real exec does not return
}

// keyFunc reads provider keys and prints the warnings about them.
func keyFunc(cfg *config.Config, sys system) launch.KeyFunc {
	r := keys.Reader{LookupEnv: sys.lookupEnv, Stdin: sys.stdin, Stderr: sys.stderr}
	return func(provider string) (string, error) {
		key, warning, err := r.Read(cfg.Providers[provider].Key)
		if warning != "" {
			fmt.Fprintf(sys.stderr, "graft: warning: %s\n", warning)
		}
		if err != nil {
			return "", fmt.Errorf("provider %s: %w", provider, err)
		}
		return key, nil
	}
}

// printPlan shows what graft would start. It reads the keys, to check them
// and show their fingerprints, but never prints a key.
func printPlan(cfg *config.Config, p *plan.Plan, key launch.KeyFunc, sys system) int {
	path, err := sys.lookPath("claude")
	if err != nil {
		fmt.Fprintln(sys.stderr, "graft: claude not found in PATH")
		return 1
	}
	var set []string
	for _, e := range p.Set {
		val := e.Value
		if e.KeyOf != "" {
			k, err := key(e.KeyOf)
			if err != nil {
				fmt.Fprintf(sys.stderr, "graft: %v\n", err)
				return 1
			}
			val = fmt.Sprintf("<%s, sha256:%s>", keys.Describe(cfg.Providers[e.KeyOf].Key), keys.Fingerprint(k))
		}
		set = append(set, e.Name+"="+val)
	}
	settings, err := indentJSON(p.Settings)
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, "profile %s: %s", p.Profile, p.Mode)
	switch len(p.Providers) {
	case 0:
	case 1:
		fmt.Fprintf(&b, ", provider %s", p.Providers[0])
	default:
		fmt.Fprintf(&b, ", providers %s", strings.Join(p.Providers, ", "))
	}
	if cfg.Profiles[p.Profile].Subscription {
		b.WriteString(", subscription")
	}
	b.WriteString("\n")
	block(&b, "unset", p.Unset)
	block(&b, "set", set)
	block(&b, "exec", []string{shellJoin(append([]string{path}, p.Args...))})
	b.WriteString("settings\n")
	b.WriteString(settings)
	io.WriteString(sys.stdout, b.String())
	return 0
}

// block writes lines under a label that only the first line shows.
func block(b *strings.Builder, label string, lines []string) {
	for i, line := range lines {
		if i > 0 {
			label = ""
		}
		fmt.Fprintf(b, "%-7s%s\n", label, line)
	}
}

// indentJSON encodes v for people: indented, without HTML escaping.
func indentJSON(v any) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return b.String(), nil
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellJoin quotes args for a POSIX shell, so that the line can be pasted.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if shellSafe.MatchString(a) {
			quoted[i] = a
		} else {
			quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(quoted, " ")
}
