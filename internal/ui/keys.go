package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/edit"
	"github.com/zinin/graft/internal/keys"
)

// errNotRun is the answer for a key command that has not run in this
// session: it runs only when the user asks, since it may ask for a password.
var errNotRun = errors.New("the key command has not run yet: press Run at the key of the provider; the command may ask for a password in the terminal of graft ui")

type keyCheckRequest struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Run   bool   `json:"run"` // whether a key command may run now
}

// keyState is what the key widget shows of a key source. The key itself
// never leaves the server.
type keyState struct {
	State       string `json:"state"`                 // ok, missing, unset, not run or error
	Path        string `json:"path,omitempty"`        // the key file graft reads, shown with ~
	Fingerprint string `json:"fingerprint,omitempty"` // the first 12 hex digits of the key's SHA-256
	Warning     string `json:"warning,omitempty"`
	Error       string `json:"error,omitempty"`
}

type keyWriteRequest struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
	Replace  bool   `json:"replace"`
}

type keyWritten struct {
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

// keyConflict refuses to write over a key file without replace.
type keyConflict struct {
	Error       string `json:"error"`
	Fingerprint string `json:"fingerprint"` // of the key in the file, "" when graft cannot read one there
}

func (a *api) keyCheck(w http.ResponseWriter, r *http.Request) {
	var req keyCheckRequest
	if !readJSON(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, a.keyState(edit.Key{Kind: req.Kind, Value: req.Value}, req.Run))
}

func (a *api) keyWrite(w http.ResponseWriter, r *http.Request) {
	var req keyWriteRequest
	if !readJSON(w, r, &req) {
		return
	}
	// The name becomes the name of a file: the rules of a provider name keep
	// it inside keys/.
	if msg := config.CheckProviderName(req.Provider); msg != "" {
		a.fail(w, http.StatusUnprocessableEntity, errors.New(msg))
		return
	}
	key, err := keys.Clean(req.Key)
	if err != nil {
		a.fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	path := filepath.Join(filepath.Dir(a.path), "keys", req.Provider)
	shown := config.Tildify(path)
	a.keyMu.Lock()
	defer a.keyMu.Unlock()
	if _, err := os.Lstat(path); err == nil && !req.Replace {
		// The fingerprint tells the user which key the file holds, so that
		// the page can ask before it replaces one.
		conflict := keyConflict{Error: shown + " already holds a key"}
		if old, _, err := a.keys.Read(config.KeySource{File: path}); err == nil {
			conflict.Fingerprint = keys.Fingerprint(old)
		}
		writeJSON(w, http.StatusConflict, conflict)
		return
	}
	if err := writeKeyFile(path, key); err != nil {
		fmt.Fprintf(a.log, "graft ui: cannot write %s: %v\n", shown, err)
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	fp := keys.Fingerprint(key)
	fmt.Fprintf(a.log, "graft ui: wrote %s (sha256:%s)\n", shown, fp)
	writeJSON(w, http.StatusOK, keyWritten{Path: shown, Fingerprint: fp})
}

// source returns the key source k names, as config.Parse reads it: a file
// with a leading ~ expanded, and relative to the directory of the config.
func (a *api) source(k edit.Key) (config.KeySource, error) {
	if strings.TrimSpace(k.Value) == "" {
		return config.KeySource{}, errors.New("must not be empty")
	}
	switch k.Kind {
	case "env":
		return config.KeySource{Env: k.Value}, nil
	case "file":
		path, err := config.ResolvePath(k.Value, filepath.Dir(a.path))
		if err != nil {
			return config.KeySource{}, err
		}
		return config.KeySource{File: path}, nil
	case "command":
		return config.KeySource{Command: k.Value}, nil
	}
	return config.KeySource{}, errors.New("say where the key comes from: env, file or command")
}

// keyState reads the key of k. A key command runs only when run is set.
func (a *api) keyState(k edit.Key, run bool) keyState {
	src, err := a.source(k)
	if err != nil {
		return keyState{State: "error", Error: err.Error()}
	}
	var st keyState
	switch {
	case src.File != "":
		st.Path = config.Tildify(src.File)
		if _, err := os.Stat(src.File); errors.Is(err, fs.ErrNotExist) {
			st.State = "missing"
			return st
		}
	case src.Env != "":
		// The lookup of the reader: unset here is what Read would call not
		// set.
		if _, ok := a.keys.LookupEnv(src.Env); !ok {
			st.State = "unset"
			return st
		}
	}
	key, warning, err := a.readKey(src, run)
	switch {
	case errors.Is(err, errNotRun):
		st.State = "not run"
	case err != nil:
		st.State, st.Error, st.Warning = "error", err.Error(), warning
	default:
		st.State, st.Fingerprint, st.Warning = "ok", keys.Fingerprint(key), warning
	}
	return st
}

// readKey reads the key of src. A key command runs only when run is set,
// one at a time, since each may ask for a password in the terminal, and its
// key is kept for the session by the text of the command: so a test request
// does not ask for the password again. Without run, the key is the one kept,
// or errNotRun. A failed run drops the kept key. Env and file do not wait
// for a command.
func (a *api) readKey(src config.KeySource, run bool) (key, warning string, err error) {
	if src.Command == "" {
		return a.keys.Read(src)
	}
	if !run {
		// keptMu, not cmdMu: a check does not wait for a command that waits
		// for a password.
		a.keptMu.Lock()
		defer a.keptMu.Unlock()
		if key, ok := a.kept[src.Command]; ok {
			return key, "", nil
		}
		return "", "", errNotRun
	}
	a.cmdMu.Lock()
	defer a.cmdMu.Unlock()
	key, warning, err = a.keys.Read(src)
	// The kept key changes before the next run takes its turn, so it is
	// always what the last run gave.
	a.keptMu.Lock()
	if err == nil {
		a.kept[src.Command] = key
	} else {
		delete(a.kept, src.Command)
	}
	a.keptMu.Unlock()
	return key, warning, err
}

// writeKeyFile puts key into the file at path, readable by its owner only,
// in a directory made with mode 0700 when there is none: a temporary file,
// synced, renamed over the file.
func writeKeyFile(path, key string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := writeTemp(path, []byte(key+"\n"), 0o600)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	syncDir(dir)
	return nil
}
