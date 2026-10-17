package launch

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/zinin/graft/internal/plan"
)

// PlanText returns what graft --dry-run prints for plan p: the mode and the
// providers, the variables graft unsets and sets, the routes, the command
// line with claude at path claude, and the settings. show returns what
// stands in for the key of a provider; no key is ever shown.
func PlanText(p *plan.Plan, claude string, show func(provider string) (string, error)) (string, error) {
	var set, routes []string
	for _, e := range p.Set {
		val := e.Value
		switch {
		case e.KeyOf != "":
			shown, err := show(e.KeyOf)
			if err != nil {
				return "", err
			}
			val = shown
		case e.Session != 0:
			val = SessionValue(e.Session, "<port>", "<session secret>")
		}
		set = append(set, e.Name+"="+val)
	}
	for _, r := range p.Routes {
		shown, err := show(r.Provider)
		if err != nil {
			return "", err
		}
		routes = append(routes, fmt.Sprintf("%s -> %s %s %s", r.Model, r.Provider, r.ID, shown))
	}
	settings, err := indentJSON(p.Settings)
	if err != nil {
		return "", err
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
	block(&b, "exec", []string{shellJoin(append([]string{claude}, p.Args...))})
	b.WriteString("settings\n")
	b.WriteString(settings)
	return b.String(), nil
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
