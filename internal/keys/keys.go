// Package keys reads provider keys from the sources named in the config.
package keys

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
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
		if info.Mode().Perm()&0o044 != 0 {
			warning = fmt.Sprintf("key file %s is readable by other users: run chmod 600 on it", config.Tildify(src.File))
		}
		data, err := os.ReadFile(src.File)
		if err != nil {
			return "", warning, fmt.Errorf("%s: %w", Describe(src), unwrapPath(err))
		}
		raw = string(data)
	case src.Command != "":
		cmd := exec.Command("/bin/sh", "-c", src.Command)
		cmd.Stdin, cmd.Stderr = r.Stdin, r.Stderr
		out, err := cmd.Output()
		if err != nil {
			return "", "", fmt.Errorf("%s failed: %w", Describe(src), err)
		}
		raw = string(out)
	default:
		return "", "", errors.New("no key source")
	}
	if key = strings.TrimSpace(raw); key == "" {
		return "", warning, fmt.Errorf("%s: the key is empty", Describe(src))
	}
	if msg := badChar(key); msg != "" {
		return "", warning, fmt.Errorf("%s: %s", Describe(src), msg)
	}
	return key, warning, nil
}

// badChar says why key cannot go into an HTTP header, or returns "": a key
// is one line of printable ASCII characters. An invisible character, such
// as the byte order mark some editors write, would break the header.
func badChar(key string) string {
	for _, r := range key {
		switch {
		case r > ' ' && r < 0x7f:
		case unicode.IsSpace(r) || unicode.IsControl(r):
			return "the key must be one line without spaces"
		default:
			return fmt.Sprintf("the key contains %U, which is not a printable ASCII character", r)
		}
	}
	return ""
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
