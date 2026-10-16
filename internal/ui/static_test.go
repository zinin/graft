package ui

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

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

// TestPageLoadsItsFiles checks that index.html and the modules load only
// files that exist, and that the page has no inline script or style, which
// the CSP would block.
func TestPageLoadsItsFiles(t *testing.T) {
	page, err := static.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(string(page), -1) {
		if strings.HasPrefix(m[1], "data:") {
			continue
		}
		if _, err := static.ReadFile("static/" + m[1]); err != nil {
			t.Errorf("index.html loads %s: %v", m[1], err)
		}
	}
	for _, bad := range []string{"<style", " style=", " onclick=", " onload="} {
		if strings.Contains(string(page), bad) {
			t.Errorf("index.html has %q, which the CSP blocks", bad)
		}
	}
	for _, m := range regexp.MustCompile(`<script[^>]*>`).FindAllString(string(page), -1) {
		if !strings.Contains(m, " src=") {
			t.Errorf("inline script %s", m)
		}
	}
	imports := regexp.MustCompile(`from '\./([^']+)'`)
	modules, _ := fs.Glob(static, "static/*.js")
	if len(modules) == 0 {
		t.Fatal("no modules")
	}
	for _, name := range modules {
		data, _ := static.ReadFile(name)
		for _, m := range imports.FindAllStringSubmatch(string(data), -1) {
			if _, err := static.ReadFile("static/" + m[1]); err != nil {
				t.Errorf("%s imports %s: %v", name, m[1], err)
			}
		}
	}
}
