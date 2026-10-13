package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/keys"
	"github.com/zinin/graft/internal/plan"
)

// minClaude is the oldest Claude Code that takes every setting graft writes:
// autoCompactWindow per model arrived in 2.1.288.
const minClaude = "2.1.288"

// check reports on the config, the key sources, claude and the Claude Code
// settings files. It returns 1 when something would stop a launch.
func check(cfg *config.Config, sys system) int {
	w := sys.stdout
	failed := false
	fmt.Fprintf(w, "config %s: ok\n", config.Tildify(cfg.Path))

	r := keys.Reader{LookupEnv: sys.lookupEnv, Stdin: sys.stdin, Stderr: sys.stderr}
	for _, name := range cfg.ProviderOrder {
		src := cfg.Providers[name].Key
		key, warning, err := r.Read(src)
		if warning != "" {
			fmt.Fprintf(w, "provider %s: warning: %s\n", name, warning)
		}
		if err != nil {
			fmt.Fprintf(w, "provider %s: error: %v\n", name, err)
			failed = true
			continue
		}
		fmt.Fprintf(w, "provider %s: key ok (%s, sha256:%s)\n", name, keys.Describe(src), keys.Fingerprint(key))
	}

	if path, err := sys.lookPath("claude"); err != nil {
		fmt.Fprintln(w, "claude: error: not found in PATH")
		failed = true
	} else if v, err := sys.claudeVersion(path); err != nil {
		fmt.Fprintf(w, "claude: warning: cannot read the version of %s: %v\n", path, err)
	} else if olderThan(v, minClaude) {
		fmt.Fprintf(w, "claude: warning: %s is version %s; graft needs %s or later\n", path, v, minClaude)
	} else {
		fmt.Fprintf(w, "claude: %s %s\n", path, v)
	}

	// A profile with its own CLAUDE_CONFIG_DIR is checked when it starts.
	if userDir, err := plan.UserSettingsDir(cfg, "", sys.environ, sys.home); err != nil {
		fmt.Fprintf(w, "claude settings: warning: %v\n", err)
	} else {
		files, warnings := plan.LoadClaudeSettings(userDir, sys.cwd)
		for _, warning := range warnings {
			fmt.Fprintf(w, "claude settings: warning: %s\n", warning)
		}
		if found := plan.Conflicts(files); len(found) > 0 {
			fmt.Fprintf(w, "claude settings: warning: profiles with external models will not start here because of %s\n",
				strings.Join(found, ", "))
		}
	}

	for _, name := range cfg.ProfileOrder {
		if cfg.ModeOf(cfg.Profiles[name]) == config.Router {
			fmt.Fprintf(w, "profile %s: needs the router, which is not implemented yet\n", name)
		}
	}
	if failed {
		return 1
	}
	return 0
}

// claudeVersion runs claude --version and returns its first word, as in
// "2.1.296 (Claude Code)".
func claudeVersion(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", errors.New("empty output")
	}
	return fields[0], nil
}

// olderThan compares dotted version numbers; a part that is not a number
// counts as 0.
func olderThan(v, minimum string) bool {
	a, b := strings.Split(v, "."), strings.Split(minimum, ".")
	for i := range max(len(a), len(b)) {
		if x, y := versionPart(a, i), versionPart(b, i); x != y {
			return x < y
		}
	}
	return false
}

func versionPart(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, _ := strconv.Atoi(parts[i])
	return n
}
