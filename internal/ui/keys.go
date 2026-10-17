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
	// The page asks before it replaces what is there: without replace, a path
	// that exists gets 409 at once, with no temporary file written. keyMu
	// keeps out the writes of this process only, so writeKeyFile refuses a
	// file that another process makes after this check, and the answer is the
	// same.
	if _, err := os.Lstat(path); err == nil && !req.Replace {
		writeJSON(w, http.StatusConflict, a.conflict(path))
		return
	}
	if err := writeKeyFile(path, key, req.Replace); err != nil {
		// Only without replace: with it, os.Rename reports a directory in the
		// way as fs.ErrExist too, and that write failed.
		if !req.Replace && errors.Is(err, fs.ErrExist) {
			writeJSON(w, http.StatusConflict, a.conflict(path))
			return
		}
		fmt.Fprintf(a.log, "graft ui: cannot write %s: %v\n", shown, err)
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	fp := keys.Fingerprint(key)
	fmt.Fprintf(a.log, "graft ui: wrote %s (sha256:%s)\n", shown, fp)
	writeJSON(w, http.StatusOK, keyWritten{Path: shown, Fingerprint: fp})
}

// conflict is the answer to a write without replace to path, where something
// is. Only when graft reads a key there does the answer say that the file
// holds a key, with its fingerprint to tell the user which one; else it says
// that the path exists. readKey reads only a regular file: a FIFO here would
// hold keyMu, and every later write with it.
func (a *api) conflict(path string) keyConflict {
	shown := config.Tildify(path)
	c := keyConflict{Error: shown + " already exists"}
	if old, _, err := a.readKey(config.KeySource{File: path}, false); err == nil {
		c.Error, c.Fingerprint = shown+" already holds a key", keys.Fingerprint(old)
	}
	return c
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

// maxKeyFile is the size of the largest key file graft ui reads.
const maxKeyFile = 64 << 10

// readKey reads the key of src. A key file must be a regular file: the page
// reads keys by itself, after each edit, and a FIFO or a terminal would hold
// the read until someone writes to it, while /dev/zero would feed it without
// end. A key file must also be no larger than maxKeyFile, since each read
// takes it whole. A key command runs only when run is set, one at a time,
// since each may ask for a password in the terminal, and its key is kept
// for the session by the text of the command: so a test request does not
// ask for the password again. Without run, the key is the one kept, or
// errNotRun. A failed run drops the kept key. Env and file do not wait for
// a command.
func (a *api) readKey(src config.KeySource, run bool) (key, warning string, err error) {
	if src.File != "" {
		// The check is here, not in package keys: graft reads a key once,
		// when the user starts a profile, and from any file the user names.
		if info, err := os.Stat(src.File); err == nil {
			switch {
			case !info.Mode().IsRegular():
				return "", "", fmt.Errorf("%s: not a regular file", keys.Describe(src))
			case info.Size() > maxKeyFile:
				return "", "", fmt.Errorf("%s: larger than 64 KiB, too large for a key file", keys.Describe(src))
			}
		}
	}
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

// errKeyFileExists is the error of writeKeyFile without replace for a path
// where something is.
var errKeyFileExists = fmt.Errorf("key file: %w", fs.ErrExist)

// link is os.Link; a test puts a failure in its place to see writeKeyFile on
// a file system without hard links.
var link = os.Link

// writeKeyFile puts key into the file at path, readable by its owner only,
// in a directory made with mode 0700 when there is none: a temporary file,
// synced, then put at path. With replace, a rename puts it over whatever is
// there. Without, a hard link puts it at path, and the link fails when
// anything is there, whichever process made it: no file is replaced, and the
// error is errKeyFileExists. Where the link fails for another reason, as on
// a file system without hard links, the temporary file goes and
// createKeyFile makes the key file at path in a step that fails in the same
// way: a check that nothing is there and then a rename would replace a file
// that another process made between the two, and an error would leave Save
// key unable to write a key file on such a file system. The link stays the
// first way, since the file it puts at path is whole, as after a rename,
// while the file of createKeyFile is empty until the key is written.
func writeKeyFile(path, key string, replace bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data := []byte(key + "\n")
	tmp, err := writeTemp(path, data, 0o600)
	if err != nil {
		return err
	}
	if !replace {
		err := link(tmp, path)
		// The file is at path now, or it does not go there: either way, its
		// temporary name goes.
		os.Remove(tmp)
		if errors.Is(err, fs.ErrExist) {
			return errKeyFileExists
		}
		if err != nil {
			// No hard links here.
			if err := createKeyFile(path, data); err != nil {
				return err
			}
		}
		syncDir(dir)
		return nil
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	syncDir(dir)
	return nil
}

// createKeyFile makes the file at path with data in it, mode 0600, synced.
// os.O_EXCL makes the file in the same step that finds nothing at path: it
// fails when anything is there, a dangling symbolic link included, whichever
// process made it, and the error is then errKeyFileExists. A file that it
// made but could not fill goes again.
func createKeyFile(path string, data []byte) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return errKeyFileExists
	}
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	// The mode of OpenFile passes through the umask.
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
