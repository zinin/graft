package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/edit"
	"github.com/zinin/graft/internal/keys"
)

// applyDraft is edit.Apply; a test puts a fault in its place to see that
// a save stops when the file would differ from the page.
var applyDraft = edit.Apply

// api serves /api/: the draft of the config, its checks and its saves, the
// presets, the keys of providers, the test request and Preview.
type api struct {
	path  string // the config file, absolute
	store *Store
	log   io.Writer // the terminal
	opts  Options
	keys  keys.Reader // reads keys from the environment and the terminal of graft ui

	mu sync.Mutex // one save at a time

	keyMu  sync.Mutex        // one write of a key file at a time
	cmdMu  sync.Mutex        // one key command at a time: each may ask for a password
	keptMu sync.Mutex        // guards kept
	kept   map[string]string // the keys that key commands printed, by the text of the command
}

// newAPI returns the API of the config file opts.Path.
func newAPI(opts Options) *api {
	return &api{
		path:  opts.Path,
		store: NewStore(opts.Path),
		log:   opts.Log,
		opts:  opts,
		keys:  keys.Reader{LookupEnv: lookupIn(opts.Environ), Stdin: opts.Stdin, Stderr: opts.Stderr},
		kept:  map[string]string{},
	}
}

// lookupIn finds a variable in environ, as os.LookupEnv does in the
// environment of the process.
func lookupIn(environ []string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		// The first entry of a name wins, as for os.LookupEnv and for graft
		// when it reads the key of a profile.
		for _, kv := range environ {
			if k, v, ok := strings.Cut(kv, "="); ok && k == name {
				return v, true
			}
		}
		return "", false
	}
}

func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config", a.config)
	mux.HandleFunc("GET /api/version", a.version)
	mux.HandleFunc("POST /api/validate", a.validate)
	mux.HandleFunc("POST /api/save", a.save)
	mux.HandleFunc("GET /api/presets", a.presets)
	mux.HandleFunc("POST /api/key/check", a.keyCheck)
	mux.HandleFunc("POST /api/key/write", a.keyWrite)
}

// fileState is the config file as the page opens it.
type fileState struct {
	Path     string      `json:"path"`     // as shown, with ~
	Exists   bool        `json:"exists"`   // false: the page starts from an empty config
	Version  string      `json:"version"`  // "" when the file does not exist
	Editable bool        `json:"editable"` // whether the forms can show the file
	Draft    *edit.Draft `json:"draft"`    // nil when not editable
	Problems []problem   `json:"problems"` // why not editable, at lines of the file
}

// problem is an error of the config: at a line of the file on disk, or at
// a path for a draft, whose lines the page never shows.
type problem struct {
	Line int    `json:"line,omitempty"`
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

// checkResult is what the page shows after a change.
type checkResult struct {
	Stale    bool                    `json:"stale"` // the file changed: nothing was checked
	Errors   []problem               `json:"errors"`
	Profiles map[string]profileState `json:"profiles"` // empty while there are errors
}

type profileState struct {
	Mode         string   `json:"mode"`
	Providers    []string `json:"providers"`
	Subscription bool     `json:"subscription"`
}

// draftRequest is a draft and the version of the file it is based on.
type draftRequest struct {
	Version string     `json:"version"`
	Draft   edit.Draft `json:"draft"`
}

type savedFile struct {
	Version string     `json:"version"`
	Draft   edit.Draft `json:"draft"`
}

type errorBody struct {
	Error string `json:"error"`
}

func (a *api) config(w http.ResponseWriter, r *http.Request) {
	data, ver, err := a.store.Read()
	if err != nil {
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	st := fileState{Path: config.Tildify(a.path), Exists: ver != "", Version: ver, Problems: []problem{}}
	d, err := edit.Read(data)
	var se *edit.ShapeError
	switch {
	case errors.As(err, &se):
		st.Problems = a.fileProblems(data, se)
	case err != nil:
		a.fail(w, http.StatusInternalServerError, err)
		return
	default:
		st.Editable, st.Draft = true, &d
	}
	writeJSON(w, http.StatusOK, st)
}

// fileProblems lists what is wrong with a file the forms cannot show: the
// errors of config.Parse, which have lines, or the shape problems when
// Parse finds none.
func (a *api) fileProblems(data []byte, se *edit.ShapeError) []problem {
	var out []problem
	var errs config.Errors
	if _, err := config.Parse(data, a.path); errors.As(err, &errs) {
		for _, e := range errs {
			out = append(out, problem{Line: e.Line, Path: e.Path, Msg: e.Msg})
		}
		return out
	}
	for _, p := range se.Problems {
		out = append(out, problem{Line: p.Line, Path: p.Path, Msg: p.Msg})
	}
	return out
}

func (a *api) version(w http.ResponseWriter, r *http.Request) {
	v, err := a.store.Version()
	if err != nil {
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": v})
}

func (a *api) validate(w http.ResponseWriter, r *http.Request) {
	var req draftRequest
	if !readJSON(w, r, &req) {
		return
	}
	data, ver, err := a.store.Read()
	if err != nil {
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	if ver != req.Version {
		writeJSON(w, http.StatusOK, checkResult{Stale: true, Errors: []problem{}, Profiles: map[string]profileState{}})
		return
	}
	_, res := a.check(data, req.Draft)
	writeJSON(w, http.StatusOK, res)
}

func (a *api) save(w http.ResponseWriter, r *http.Request) {
	var req draftRequest
	if !readJSON(w, r, &req) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	data, ver, err := a.store.Read()
	if err != nil {
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	if ver != req.Version {
		a.fail(w, http.StatusConflict, ErrConflict)
		return
	}
	out, res := a.check(data, req.Draft)
	if out == nil {
		writeJSON(w, http.StatusUnprocessableEntity, res)
		return
	}
	// The file must hold what the page shows: a fault in edit must not
	// reach the disk.
	back, err := edit.Read(out)
	if err != nil || !edit.Same(back, req.Draft) {
		fmt.Fprintf(a.log, "graft ui: a save did not come out as the page showed it, so %s stays as it was\n", config.Tildify(a.path))
		a.fail(w, http.StatusInternalServerError, errors.New("graft ui could not apply the changes as shown, so the file is unchanged: reload the page"))
		return
	}
	newVer, backup, err := a.store.Write(ver, out)
	switch {
	case errors.Is(err, ErrConflict):
		a.fail(w, http.StatusConflict, err)
		return
	case err != nil:
		fmt.Fprintf(a.log, "graft ui: cannot save %s: %v\n", config.Tildify(a.path), err)
		a.fail(w, http.StatusInternalServerError, err)
		return
	case backup == "":
		fmt.Fprintf(a.log, "graft ui: created %s\n", config.Tildify(a.path))
	default:
		fmt.Fprintf(a.log, "graft ui: saved %s (previous version in %s)\n", config.Tildify(a.path), config.Tildify(backup))
	}
	writeJSON(w, http.StatusOK, savedFile{Version: newVer, Draft: back})
}

// check applies d to data and checks the result as graft would. It returns
// the new file, nil when d has errors, and what the page shows.
func (a *api) check(data []byte, d edit.Draft) ([]byte, checkResult) {
	res := checkResult{Errors: []problem{}, Profiles: map[string]profileState{}}
	out, err := applyDraft(data, d)
	if err == nil {
		var cfg *config.Config
		if cfg, err = config.Parse(out, a.path); err == nil {
			for _, name := range cfg.ProfileOrder {
				p := cfg.Profiles[name]
				providers := cfg.ProvidersOf(p)
				if providers == nil {
					providers = []string{}
				}
				res.Profiles[name] = profileState{Mode: cfg.ModeOf(p).String(), Providers: providers, Subscription: p.Subscription}
			}
			return out, res
		}
	}
	res.Errors = draftProblems(err)
	return nil, res
}

// draftProblems turns the errors of edit.Apply or config.Parse into
// problems at paths.
func draftProblems(err error) []problem {
	var out []problem
	var ee edit.Errors
	var ce config.Errors
	var se *edit.ShapeError
	switch {
	case errors.As(err, &ee):
		for _, e := range ee {
			out = append(out, problem{Path: e.Path, Msg: e.Msg})
		}
	case errors.As(err, &ce):
		for _, e := range ce {
			out = append(out, problem{Path: e.Path, Msg: e.Msg})
		}
	case errors.As(err, &se):
		for _, p := range se.Problems {
			out = append(out, problem{Path: p.Path, Msg: p.Msg})
		}
	default:
		out = append(out, problem{Msg: err.Error()})
	}
	return out
}

func (a *api) fail(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorBody{err.Error()})
}

// readJSON decodes the body of r into v; on failure it answers 400, or 413
// for a body over the limit, and returns false.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(r.Body).Decode(v)
	if err == nil {
		return true
	}
	status := http.StatusBadRequest
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		status = http.StatusRequestEntityTooLarge
	}
	writeJSON(w, status, errorBody{"graft ui: bad request: " + err.Error()})
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
