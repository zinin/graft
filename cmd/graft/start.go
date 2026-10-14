package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	key := keyFunc(cfg, sys)
	switch {
	case dryRun:
		return printPlan(cfg, p, key, sys)
	case p.Mode == config.Router:
		return startRouter(cfg, p, plan.PlainSettingsDir(sys.environ, sys.home), key, sys)
	}
	if err := launch.Exec(p, sys.environ, key, sys.lookPath, sys.exec); err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	return 0 // only in tests: a real exec does not return
}

// startRouter runs claude behind the router and returns claude's exit code.
// Without a log the router still runs. Once claude has exited, it warns if
// the user settings in plainDir, those of a plain claude, now save a graft
// model for new sessions.
func startRouter(cfg *config.Config, p *plan.Plan, plainDir string, key launch.KeyFunc, sys system) int {
	rc := launch.RunConfig{LookPath: sys.lookPath, Stdin: sys.stdin, Stdout: sys.stdout, Stderr: sys.stderr}
	path, err := logPath(sys)
	var f *os.File
	if err == nil {
		f, err = openLog(path)
	}
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: warning: cannot open the log, so the router keeps none: %v\n", err)
	} else {
		defer f.Close()
		rc.Log = f
	}
	code, err := launch.Run(p, sys.environ, key, rc)
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	if warning := savedModelWarning(cfg, plainDir); warning != "" {
		fmt.Fprintf(sys.stderr, "graft: warning: %s\n", warning)
	}
	return code
}

// keyFunc reads provider keys and prints the warnings about them. It reads
// each key once: a key command may ask for a password.
func keyFunc(cfg *config.Config, sys system) launch.KeyFunc {
	r := keys.Reader{LookupEnv: sys.lookupEnv, Stdin: sys.stdin, Stderr: sys.stderr}
	read := map[string]string{}
	return func(provider string) (string, error) {
		if key, ok := read[provider]; ok {
			return key, nil
		}
		key, warning, err := r.Read(cfg.Providers[provider].Key)
		if warning != "" {
			fmt.Fprintf(sys.stderr, "graft: warning: %s\n", warning)
		}
		if err != nil {
			return "", fmt.Errorf("provider %s: %w", provider, err)
		}
		read[provider] = key
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
	var set, routes []string
	for _, e := range p.Set {
		val := e.Value
		switch {
		case e.KeyOf != "":
			if val, err = showKey(cfg, e.KeyOf, key); err != nil {
				fmt.Fprintf(sys.stderr, "graft: %v\n", err)
				return 1
			}
		case e.Session != 0:
			val = launch.SessionValue(e.Session, "<port>", "<session secret>")
		}
		set = append(set, e.Name+"="+val)
	}
	for _, r := range p.Routes {
		shown, err := showKey(cfg, r.Provider, key)
		if err != nil {
			fmt.Fprintf(sys.stderr, "graft: %v\n", err)
			return 1
		}
		routes = append(routes, fmt.Sprintf("%s -> %s %s %s", r.Model, r.Provider, r.ID, shown))
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
	if p.Subscription {
		b.WriteString(", subscription")
	}
	b.WriteString("\n")
	block(&b, "unset", p.Unset)
	block(&b, "set", set)
	block(&b, "route", routes)
	block(&b, "exec", []string{shellJoin(append([]string{path}, p.Args...))})
	b.WriteString("settings\n")
	b.WriteString(settings)
	io.WriteString(sys.stdout, b.String())
	return 0
}

// showKey reads the provider's key and shows it as its source and the start
// of its SHA-256.
func showKey(cfg *config.Config, provider string, key launch.KeyFunc) (string, error) {
	k, err := key(provider)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("<%s, sha256:%s>", keys.Describe(cfg.Providers[provider].Key), keys.Fingerprint(k)), nil
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
