package plan

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/osenv"
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
// the env of defaults, else from environ, else .claude in the home directory
// claude gets: the variable of osenv.HomeVar (HOME, USERPROFILE on Windows)
// from the profile's env, else from the env of defaults, else home. An empty
// one in an env means no home directory, and without one it fails rather
// than guess: Claude Code would still find one, and graft must check the
// same file.
func UserSettingsDir(cfg *config.Config, profile string, environ []string, home string) (string, error) {
	homeVar := osenv.HomeVar()
	dir := lookup(environ, "CLAUDE_CONFIG_DIR")
	if v, ok := envEntry(cfg.Defaults.Env, "CLAUDE_CONFIG_DIR"); ok {
		dir = v
	}
	if v, ok := envEntry(cfg.Defaults.Env, homeVar); ok {
		home = v
	}
	if p, ok := cfg.Profiles[profile]; ok {
		if v, ok := envEntry(p.Env, "CLAUDE_CONFIG_DIR"); ok {
			dir = v
		}
		if v, ok := envEntry(p.Env, homeVar); ok {
			home = v
		}
	}
	if dir != "" {
		return dir, nil
	}
	if home == "" {
		return "", errors.New("cannot find the home directory, so graft cannot check the Claude Code settings: set " + homeVar)
	}
	return filepath.Join(home, ".claude"), nil
}

// envEntry returns the value of the variable name in env, a config's env;
// see envKey.
func envEntry(env map[string]string, name string) (string, bool) {
	k, ok := envKey(env, name)
	return env[k], ok
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
		if err := json.Unmarshal(settingsText(data), &obj); err != nil {
			warnings = append(warnings, fmt.Sprintf("cannot parse %s, so it was not checked: %v", shown, err))
			continue
		}
		files = append(files, SettingsFile{Path: shown, Data: obj})
	}
	return files, warnings
}

// settingsText returns the JSON text of a settings file that holds data.
// Claude Code reads a file that starts with the UTF-8 byte order mark, and
// one in UTF-16 little-endian with its mark, and applies its env; Windows
// PowerShell 5.1 writes both, the first with Set-Content -Encoding UTF8, the
// second with > and Out-File. graft must check such a file rather than skip
// it as broken. Anything else comes back as it is and fails to parse as
// before: UTF-16 with an odd number of bytes, and UTF-16 big-endian, whose
// env Claude Code does not apply.
func settingsText(data []byte) []byte {
	if rest, ok := bytes.CutPrefix(data, []byte{0xef, 0xbb, 0xbf}); ok {
		return rest
	}
	if !bytes.HasPrefix(data, []byte{0xff, 0xfe}) || len(data)%2 != 0 {
		return data
	}
	units := make([]uint16, len(data)/2-1)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[2+2*i:])
	}
	return []byte(string(utf16.Decode(units)))
}

// LoadSettingsFor returns the Claude Code settings files that matter when
// profile starts in directory dir: those of LoadClaudeSettings, for the user
// settings of UserSettingsDir. They matter only to a profile with external
// models, so for any other there are none, and it needs no home directory.
// warnings name the files that could not be read.
func LoadSettingsFor(cfg *config.Config, profile string, environ []string, home, dir string) (files []SettingsFile, warnings []string, err error) {
	prof, ok := cfg.Profiles[profile]
	if !ok || len(cfg.ExternalModels(prof)) == 0 {
		return nil, nil, nil
	}
	userDir, err := UserSettingsDir(cfg, profile, environ, home)
	if err != nil {
		return nil, nil, err
	}
	files, warnings = LoadClaudeSettings(userDir, dir)
	return files, warnings, nil
}

// Conflicts lists, as "file: key", the keys in files that would override the
// address or the key graft passes in the environment. A key counts whatever
// its value: an empty one cancels graft's variable, and Claude Code would
// then send the claude.ai login to the provider's address. On Windows,
// where Claude Code reads a variable in any case, a key counts in any case
// too; and so does another spelling of a variable that --settings cancels,
// which it cancels only as it writes it.
func Conflicts(files []SettingsFile) []string {
	var out []string
	for _, f := range files {
		if _, ok := f.Data["apiKeyHelper"]; ok {
			out = append(out, f.Path+": apiKeyHelper")
		}
		env, _ := f.Data["env"].(map[string]any)
		keys := slices.Sorted(maps.Keys(env))
		for _, name := range addressAndKeys {
			for _, k := range keys {
				if osenv.Equal(k, name) {
					out = append(out, f.Path+": env."+k)
				}
			}
		}
		for _, name := range pinnedEmpty {
			for _, k := range keys {
				if k != name && osenv.Equal(k, name) {
					out = append(out, f.Path+": env."+k)
				}
			}
		}
	}
	return out
}

// PlainSettingsDir returns the directory of the user settings that a plain
// claude, started from graft's shell, reads: CLAUDE_CONFIG_DIR of environ,
// else ~/.claude; "" when neither is known. The env of a profile or of the
// defaults reaches only graft's sessions.
func PlainSettingsDir(environ []string, home string) string {
	if dir := lookup(environ, "CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// SavedModel returns the model that the user settings in userDir save for
// new sessions, and the file as messages show it, when only graft serves
// that model: the name or the ID of a model of cfg, not a Claude model.
// Enter in /model saves it there, and a plain claude would then ask
// Anthropic for it. Without such a model it returns "".
func SavedModel(cfg *config.Config, userDir string) (model, file string) {
	if userDir == "" {
		return "", ""
	}
	path := filepath.Join(userDir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	var obj map[string]any
	if json.Unmarshal(settingsText(data), &obj) != nil {
		return "", ""
	}
	saved, _ := obj["model"].(string)
	if saved == "" || slices.Contains(config.AliasNames, saved) || strings.HasPrefix(strings.ToLower(saved), "claude") {
		return "", ""
	}
	for _, m := range cfg.Models {
		if saved == m.Name || saved == m.ID {
			return saved, config.Tildify(path)
		}
	}
	return "", ""
}

// lookup returns the value of name in environ, as the system compares
// names; "" when there is none.
func lookup(environ []string, name string) string {
	v, _ := osenv.Lookup(environ, name)
	return v
}
