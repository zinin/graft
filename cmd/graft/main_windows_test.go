package main

import (
	"strings"
	"testing"
)

// TestConfigPathOnWindows: --config takes ~\ as ~/, and without a home
// directory the error names USERPROFILE, which Windows keeps it in.
func TestConfigPathOnWindows(t *testing.T) {
	if got, err := configPath(`~\g.yaml`, system{home: `C:\Users\u`}); err != nil || got != `C:\Users\u\g.yaml` {
		t.Errorf(`configPath(~\g.yaml) = %q, %v`, got, err)
	}
	_, err := configPath("", system{})
	if err == nil || !strings.Contains(err.Error(), "cannot find the home directory: set USERPROFILE or pass --config") {
		t.Errorf("configPath without a home: %v", err)
	}
}
