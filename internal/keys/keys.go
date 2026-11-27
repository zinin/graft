// Package keys reads provider keys from the sources named in the config.
package keys

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode"

	"github.com/zinin/graft/internal/config"
)

// Reader reads keys. A nil LookupEnv means os.LookupEnv.
type Reader struct {
	LookupEnv func(string) (string, bool) // the environment graft runs in
	Stdin     io.Reader                   // for key commands that ask for a password
	Stderr    io.Writer
}

// Read returns the key from src without white space at the ends. warning is
// set when a key file is readable by other users.
func (r Reader) Read(src config.KeySource) (key, warning string, err error) {
	var raw string
	switch {
	case src.Env != "":
		lookup := r.LookupEnv
		if lookup == nil {
			lookup = os.LookupEnv
		}
		v, ok := lookup(src.Env)
		if !ok {
			return "", "", fmt.Errorf("%s is not set", Describe(src))
		}
		raw = v
	case src.File != "":
		info, err := os.Stat(src.File)
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", Describe(src), unwrapPath(err))
		}
		// Windows has no mode bits: Go reports 0666 or 0444 for every file.
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o044 != 0 {
			warning = fmt.Sprintf("key file %s is readable by other users: run chmod 600 on it", config.Tildify(src.File))
		}
		data, err := os.ReadFile(src.File)
		if err != nil {
			return "", warning, fmt.Errorf("%s: %w", Describe(src), unwrapPath(err))
		}
		// Windows PowerShell 5.1 writes UTF-16 with > and Out-File: name the
		// cause rather than the U+FFFD that the bytes would read as.
		if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
			return "", warning, fmt.Errorf("%s: the file is in UTF-16: save it as ASCII or as UTF-8 without a byte order mark", Describe(src))
		}
		raw = string(data)
	case src.Command != "":
		args := shellArgs(runtime.GOOS == "windows", os.Getenv("SystemRoot"), src.Command)
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = commandEnv(runtime.GOOS == "windows", os.Environ())
		cmd.Stdin, cmd.Stderr = r.Stdin, r.Stderr
		out, err := cmd.Output()
		if err != nil {
			return "", "", fmt.Errorf("%s failed: %w", Describe(src), err)
		}
		raw = string(out)
	default:
		return "", "", errors.New("no key source")
	}
	if key, err = Clean(raw); err != nil {
		return "", warning, fmt.Errorf("%s: %w", Describe(src), err)
	}
	return key, warning, nil
}

// shellArgs returns the command line that runs a key command: /bin/sh -c on
// Unix. On Windows it is Windows PowerShell, which every Windows has, by its
// path in systemRoot, so that no program in PATH stands in for it; Go passes
// the command as one argument, which PowerShell reads as typed.
func shellArgs(windows bool, systemRoot, command string) []string {
	if !windows {
		return []string{"/bin/sh", "-c", command}
	}
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	return []string{systemRoot + `\System32\WindowsPowerShell\v1.0\powershell.exe`, "-NoLogo", "-NoProfile", "-Command", command}
}

// commandEnv returns the environment of a key command: nil, which is graft's
// own, on Unix. On Windows it is environ without PSModulePath, in any case
// of the name: PowerShell 7 puts its module folders first in PSModulePath,
// and Windows PowerShell started through graft would then take PowerShell
// 7's modules, which it cannot load. Without the variable Windows
// PowerShell builds its own, as Microsoft advises.
func commandEnv(windows bool, environ []string) []string {
	if !windows {
		return nil
	}
	env := make([]string, 0, len(environ))
	for _, kv := range environ {
		if name, _, _ := strings.Cut(kv, "="); !strings.EqualFold(name, "PSModulePath") {
			env = append(env, kv)
		}
	}
	return env
}

// Clean returns the key in raw, without white space at the ends, or an
// error when raw holds no key that can go into an HTTP header: a key is one
// line of printable ASCII characters. An invisible character, such as the
// byte order mark some editors write, would break the header.
func Clean(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", errors.New("the key is empty")
	}
	for _, r := range key {
		switch {
		case r > ' ' && r < 0x7f:
		case unicode.IsSpace(r) || unicode.IsControl(r):
			return "", errors.New("the key must be one line without spaces")
		default:
			return "", fmt.Errorf("the key contains %U, which is not a printable ASCII character", r)
		}
	}
	return key, nil
}

// Describe names a key source for messages: env NAME, file PATH or
// command "CMD".
func Describe(src config.KeySource) string {
	switch {
	case src.Env != "":
		return "env " + src.Env
	case src.File != "":
		return "file " + config.Tildify(src.File)
	default:
		return fmt.Sprintf("command %q", src.Command)
	}
}

// Fingerprint returns the first 12 hex digits of the key's SHA-256: enough to
// tell keys apart, useless for recovering one.
func Fingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}

// unwrapPath drops the path from a *fs.PathError: Describe already names it.
func unwrapPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
