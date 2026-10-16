package ui

import (
	"errors"
	"net/http"
	"os/exec"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/edit"
	"github.com/zinin/graft/internal/keys"
	"github.com/zinin/graft/internal/launch"
	"github.com/zinin/graft/internal/plan"
)

// previewRequest is a draft, the version of the file it is based on, and
// the profile to show.
type previewRequest struct {
	Version string     `json:"version"`
	Draft   edit.Draft `json:"draft"`
	Profile string     `json:"profile"`
}

// previewResult is what the tab Preview shows of a profile.
type previewResult struct {
	Stale    bool      `json:"stale"`    // the file changed: nothing was checked
	Errors   []problem `json:"errors"`   // of the config: there is no preview while it has any
	Warnings []string  `json:"warnings"` // about Claude Code's settings files
	Text     string    `json:"text"`     // what graft --dry-run would print, with each key as its source
	Failure  string    `json:"failure"`  // why graft would not start the profile
}

func (a *api) preview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if !readJSON(w, r, &req) {
		return
	}
	data, ver, err := a.store.Read()
	if err != nil {
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	// Lists even when empty, so that the page need not check for null.
	res := previewResult{Errors: []problem{}, Warnings: []string{}}
	if ver != req.Version {
		res.Stale = true
		writeJSON(w, http.StatusOK, res)
		return
	}
	_, cfg, checked := a.check(data, req.Draft)
	if cfg == nil {
		res.Errors = append(res.Errors, checked.Errors...)
		writeJSON(w, http.StatusOK, res)
		return
	}
	text, warnings, err := a.planText(cfg, req.Profile)
	res.Text, res.Warnings = text, append(res.Warnings, warnings...)
	if err != nil {
		res.Failure = err.Error()
	}
	writeJSON(w, http.StatusOK, res)
}

// planText returns what graft --dry-run would print for profile when it
// starts in the directory of graft ui, with its environment, and the
// warnings about Claude Code's settings files. It shows each key as its
// source and reads none. The warnings come with an error too, as graft
// prints them before it fails.
func (a *api) planText(cfg *config.Config, profile string) (string, []string, error) {
	files, warnings, err := plan.LoadSettingsFor(cfg, profile, a.opts.Environ, a.opts.Home, a.opts.Dir)
	if err != nil {
		return "", nil, err
	}
	// Without arguments for claude: the page shows what graft PROFILE starts.
	p, err := plan.Build(plan.Input{Config: cfg, Profile: profile, Environ: a.opts.Environ, ClaudeSettings: files})
	if err != nil {
		return "", warnings, err
	}
	lookPath := a.opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	claude, err := lookPath("claude")
	if err != nil {
		return "", warnings, errors.New("claude not found in PATH")
	}
	text, err := launch.PlanText(p, claude, func(provider string) (string, error) {
		// The source, not the key: reading a key could run a key command,
		// which may ask for a password in the terminal, and only Run starts
		// one.
		return "<" + keys.Describe(cfg.Providers[provider].Key) + ">", nil
	})
	return text, warnings, err
}
