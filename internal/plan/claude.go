package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zinin/graft/internal/config"
)

// addressAndKeys never go into --settings. --settings is visible in the
// process list and moves into a background session, where the environment
// may not follow: a provider address there without its key would send the
// claude.ai token to the provider.
var addressAndKeys = []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_CUSTOM_HEADERS"}

// SettingsFile is a Claude Code settings file graft checks before a launch.
type SettingsFile struct {
	Path string         // as shown in messages
	Data map[string]any // the parsed JSON object
}

// UserSettingsDir returns the directory of the user settings that claude
// will read for profile: CLAUDE_CONFIG_DIR from the profile's env, else from
// the env of defaults, else from environ, else ~/.claude. Without a home
// directory it fails rather than guess: Claude Code would still find one,
// and graft must check the same file.
func UserSettingsDir(cfg *config.Config, profile string, environ []string, home string) (string, error) {
	dir := lookup(environ, "CLAUDE_CONFIG_DIR")
	if v, ok := cfg.Defaults.Env["CLAUDE_CONFIG_DIR"]; ok {
		dir = v
	}
	if p, ok := cfg.Profiles[profile]; ok {
		if v, ok := p.Env["CLAUDE_CONFIG_DIR"]; ok {
			dir = v
		}
	}
	if dir != "" {
		return dir, nil
	}
	if home == "" {
		return "", errors.New("cannot find the home directory, so graft cannot check the Claude Code settings: set HOME")
	}
	return filepath.Join(home, ".claude"), nil
}

// LoadClaudeSettings reads the Claude Code settings files that could
// override the address or key graft passes in the environment: settings.json
// in userDir (see UserSettingsDir), and .claude/settings.json and
// .claude/settings.local.json in dir. A missing file is skipped; one that
// cannot be read or parsed is skipped with a warning.
func LoadClaudeSettings(userDir, dir string) (files []SettingsFile, warnings []string) {
	seen := map[string]bool{}
	for _, path := range []string{
		filepath.Join(userDir, "settings.json"),
		filepath.Join(dir, ".claude", "settings.json"),
		filepath.Join(dir, ".claude", "settings.local.json"),
	} {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		shown := config.Tildify(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cannot read %s: %v", shown, err))
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err != nil {
			warnings = append(warnings, fmt.Sprintf("cannot parse %s, so it was not checked: %v", shown, err))
			continue
		}
		files = append(files, SettingsFile{Path: shown, Data: obj})
	}
	return files, warnings
}

// Conflicts lists, as "file: key", the keys in files that would override the
// address or the key graft passes in the environment. A key counts whatever
// its value: an empty one cancels graft's variable, and Claude Code would
// then send the claude.ai login to the provider's address.
func Conflicts(files []SettingsFile) []string {
	var out []string
	for _, f := range files {
		if _, ok := f.Data["apiKeyHelper"]; ok {
			out = append(out, f.Path+": apiKeyHelper")
		}
		env, _ := f.Data["env"].(map[string]any)
		for _, name := range addressAndKeys {
			if _, ok := env[name]; ok {
				out = append(out, f.Path+": env."+name)
			}
		}
	}
	return out
}

// lookup returns the value of name in environ.
func lookup(environ []string, name string) string {
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}
