package ui

import (
	_ "embed"
	"net/http"
)

// presetsJSON lists the providers that the first-run screen and the dialog
// Add provider offer, with the models whose windows their documentation
// names.
//
// TODO: load the current list of models from the provider, so that a new
// model needs no release of graft. The windows will likely stay here:
// /v1/models seldom gives them.
//
//go:embed presets.json
var presetsJSON []byte

// preset is a provider of presets.json.
type preset struct {
	Title   string        `json:"title"`
	Name    string        `json:"name"`
	URL     string        `json:"url"`
	Keys    string        `json:"keys"`    // the page for keys, "" when there is none
	Docs    string        `json:"docs"`    // the provider's guide for Claude Code
	Catalog string        `json:"catalog"` // where to find the models to add by hand, "" when there is no such page
	Models  []presetModel `json:"models"`
}

type presetModel struct {
	Name    string `json:"name"` // a model name of the config
	ID      string `json:"id"`
	Context int    `json:"context"`
	Label   string `json:"label"`
}

func (a *api) presets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(presetsJSON)
}
