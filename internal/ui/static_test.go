package ui

import "testing"

// TestVendoredFiles checks that the vendored files are the releases they
// are named after: htm@3.1.1/preact/standalone.module.js and
// @picocss/pico@2.1.1/css/pico.min.css.
func TestVendoredFiles(t *testing.T) {
	want := map[string]string{
		"static/vendor/htm-preact-standalone-3.1.1.module.js": "72284e8e9079c87817145df1110f74e8a2aa040b2fc384922e18dfcb46fc1fd7",
		"static/vendor/pico-2.1.1.min.css":                    "fbc9a63fc9fc9f72d12fd7fc9806e11fa9f77ae4f9cad146b27003a1119ba3db",
	}
	for name, sum := range want {
		data, err := static.ReadFile(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := version(data); got != sum {
			t.Errorf("%s has sha256 %s, want %s", name, got, sum)
		}
	}
}
