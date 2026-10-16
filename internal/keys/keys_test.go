package keys

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

func reader(env map[string]string) Reader {
	return Reader{
		LookupEnv: func(name string) (string, bool) {
			v, ok := env[name]
			return v, ok
		},
		Stdin:  strings.NewReader(""),
		Stderr: io.Discard,
	}
}

func writeKey(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { // WriteFile's mode is filtered by umask
		t.Fatal(err)
	}
	return path
}

func TestRead(t *testing.T) {
	r := reader(map[string]string{"GOOD": " sk-env \n", "EMPTY": "  ", "ZWSP": "sk-\u200benv", "PUNCT": "sk-A1_b.c~d+e/f="})
	tests := []struct {
		name    string
		src     config.KeySource
		key     string
		warning string // substring; empty means no warning
		err     string // substring; empty means no error
	}{
		{name: "env", src: config.KeySource{Env: "GOOD"}, key: "sk-env"},
		{name: "env unset", src: config.KeySource{Env: "MISSING"}, err: "env MISSING is not set"},
		{name: "env empty", src: config.KeySource{Env: "EMPTY"}, err: "env EMPTY: the key is empty"},
		{name: "file", src: config.KeySource{File: writeKey(t, "sk-file\n", 0o600)}, key: "sk-file"},
		{name: "file readable by others", src: config.KeySource{File: writeKey(t, "sk-file", 0o644)}, key: "sk-file",
			warning: "is readable by other users: run chmod 600 on it"},
		{name: "file missing", src: config.KeySource{File: filepath.Join(t.TempDir(), "none")}, err: "no such file or directory"},
		{name: "file with two lines", src: config.KeySource{File: writeKey(t, "sk-file\n# my key\n", 0o600)},
			err: "the key must be one line without spaces"},
		{name: "file with a byte order mark", src: config.KeySource{File: writeKey(t, "\ufeffsk-file\n", 0o600)},
			err: "the key contains U+FEFF, which is not a printable ASCII character"},
		{name: "env with a zero-width space", src: config.KeySource{Env: "ZWSP"},
			err: "env ZWSP: the key contains U+200B, which is not a printable ASCII character"},
		{name: "env with punctuation", src: config.KeySource{Env: "PUNCT"}, key: "sk-A1_b.c~d+e/f="},
		{name: "command", src: config.KeySource{Command: "printf ' sk-cmd \\n'"}, key: "sk-cmd"},
		{name: "command fails", src: config.KeySource{Command: "exit 3"}, err: `command "exit 3" failed: exit status 3`},
		{name: "command prints nothing", src: config.KeySource{Command: "true"}, err: `command "true": the key is empty`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, warning, err := r.Read(tt.src)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("error %v, want it to contain %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if key != tt.key {
				t.Errorf("key %q, want %q", key, tt.key)
			}
			if (tt.warning == "" && warning != "") || !strings.Contains(warning, tt.warning) {
				t.Errorf("warning %q, want %q", warning, tt.warning)
			}
		})
	}
}

func TestCommandGetsTerminal(t *testing.T) {
	r := reader(nil)
	r.Stdin = strings.NewReader("typed-password\n")
	var stderr strings.Builder
	r.Stderr = &stderr
	key, _, err := r.Read(config.KeySource{Command: "read pw; echo prompt >&2; echo \"key-for-$pw\""})
	if err != nil {
		t.Fatal(err)
	}
	if key != "key-for-typed-password" {
		t.Errorf("key %q", key)
	}
	if stderr.String() != "prompt\n" {
		t.Errorf("stderr %q, want the command's own stderr", stderr.String())
	}
}

func TestDescribe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := map[string]config.KeySource{
		"env OLLAMA_API_KEY":         {Env: "OLLAMA_API_KEY"},
		"file ~/keys/cliproxy":       {File: filepath.Join(home, "keys", "cliproxy")},
		`command "pass show mimo/x"`: {Command: "pass show mimo/x"},
	}
	for want, src := range tests {
		if got := Describe(src); got != want {
			t.Errorf("Describe(%+v) = %q, want %q", src, got, want)
		}
	}
}

func TestFingerprint(t *testing.T) {
	// sha256("secret") = 2bb80d537b1da3e38bd30361aa855686bde0eacd7162fef6a25fe97bf527a25b
	if got := Fingerprint("secret"); got != "2bb80d537b1d" {
		t.Errorf("Fingerprint = %q", got)
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		raw, key, err string
	}{
		{raw: " sk-pasted \n", key: "sk-pasted"},
		{raw: "sk-A1_b.c~d+e/f=", key: "sk-A1_b.c~d+e/f="},
		{raw: " \n", err: "the key is empty"},
		{raw: "sk-a sk-b", err: "the key must be one line without spaces"},
		{raw: "sk-a\nsk-b", err: "the key must be one line without spaces"},
		{raw: "\ufeffsk-a", err: "the key contains U+FEFF, which is not a printable ASCII character"},
	}
	for _, tt := range tests {
		key, err := Clean(tt.raw)
		if tt.err != "" {
			if err == nil || err.Error() != tt.err {
				t.Errorf("Clean(%q) error %v, want %q", tt.raw, err, tt.err)
			}
			continue
		}
		if err != nil || key != tt.key {
			t.Errorf("Clean(%q) = %q, %v, want %q", tt.raw, key, err, tt.key)
		}
	}
}
