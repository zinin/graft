package osenv

import "testing"

func TestEqual(t *testing.T) {
	tests := []struct {
		a, b          string
		unix, windows bool
	}{
		{"PATH", "PATH", true, true},
		{"PATH", "Path", false, true},
		{"NO_PROXY", "no_proxy", false, true},
		{"HOME", "HOMEPATH", false, false},
	}
	for _, tt := range tests {
		if got := equal(false, tt.a, tt.b); got != tt.unix {
			t.Errorf("equal on Unix(%q, %q) = %v", tt.a, tt.b, got)
		}
		if got := equal(true, tt.a, tt.b); got != tt.windows {
			t.Errorf("equal on Windows(%q, %q) = %v", tt.a, tt.b, got)
		}
	}
}

func TestLookup(t *testing.T) {
	environ := []string{`Path=C:\bin`, "HOME=/h", "PATH=/usr/bin"}
	check := func(windows bool, name, want string, wantOK bool) {
		t.Helper()
		if v, ok := lookup(windows, environ, name); v != want || ok != wantOK {
			t.Errorf("lookup(windows=%v, %q) = %q, %v; want %q, %v", windows, name, v, ok, want, wantOK)
		}
	}
	check(false, "PATH", "/usr/bin", true)
	check(false, "Path", `C:\bin`, true)
	check(true, "PATH", `C:\bin`, true) // the first of any case
	check(true, "USERPROFILE", "", false)
}

func TestHomeVar(t *testing.T) {
	if homeVar(false) != "HOME" || homeVar(true) != "USERPROFILE" {
		t.Errorf("homeVar = %q, %q", homeVar(false), homeVar(true))
	}
}
