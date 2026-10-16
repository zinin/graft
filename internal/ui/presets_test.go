package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/zinin/graft/internal/config"
)

// TestPresets checks each preset as the dialog Add provider uses it: the
// config it makes of the preset, with a key file, a profile per model and
// mix, passes config.Parse.
func TestPresets(t *testing.T) {
	var file struct {
		Providers []preset `json:"providers"`
	}
	dec := json.NewDecoder(bytes.NewReader(presetsJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		t.Fatal(err)
	}
	if len(file.Providers) == 0 {
		t.Fatal("no presets")
	}
	seen := map[string]bool{}
	for _, p := range file.Providers {
		t.Run(p.Name, func(t *testing.T) {
			if seen[p.Name] {
				t.Errorf("two presets are named %s", p.Name)
			}
			seen[p.Name] = true
			if p.Title == "" || p.Docs == "" {
				t.Errorf("preset %s needs a title and docs", p.Name)
			}
			for _, link := range []string{p.Keys, p.Docs, p.Catalog} {
				if link != "" && !strings.HasPrefix(link, "https://") {
					t.Errorf("link %q is not https", link)
				}
			}
			models, profiles := map[string]any{}, map[string]any{}
			picker := []string{}
			for _, m := range p.Models {
				if strings.Contains(m.ID, "[1m]") {
					t.Errorf("model %s: the id %q has [1m]: graft sets the window itself", m.Name, m.ID)
				}
				if m.Label == "" {
					t.Errorf("model %s has no label", m.Name)
				}
				models[m.Name] = map[string]any{"provider": p.Name, "id": m.ID, "context": m.Context, "label": m.Label}
				profiles[m.Name] = map[string]any{"model": m.Name}
				picker = append(picker, m.Name)
			}
			profiles["mix"] = map[string]any{"model": "opus", "picker": picker}
			cfg := map[string]any{
				"providers": map[string]any{p.Name: map[string]any{"url": p.URL, "key": map[string]any{"file": "keys/" + p.Name}}},
				"profiles":  profiles,
			}
			if len(models) > 0 {
				cfg["models"] = models
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := config.Parse(data, "/graft/config.yaml"); err != nil {
				t.Errorf("the config of the preset:\n%s\n%v", data, err)
			}
		})
	}
}

func TestPresetsAPI(t *testing.T) {
	s := startAPI(t, "")
	var file struct {
		Providers []preset `json:"providers"`
	}
	if code := s.call(t, "GET", "/api/presets", nil, &file); code != 200 || len(file.Providers) == 0 {
		t.Fatalf("GET /api/presets: %d, %d presets", code, len(file.Providers))
	}
}
