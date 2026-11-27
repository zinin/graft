package config

import "testing"

// TestTildifyOnWindows: under the home directory a path is shown as ~/…,
// matched in any case, as Windows matches paths; every path is shown with /,
// which Windows takes too, so that a config is the same on every system.
func TestTildifyOnWindows(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\Иван Петров`)
	for in, want := range map[string]string{
		`C:\Users\Иван Петров`:                         "~",
		`C:\Users\Иван Петров\.config\graft\keys\it's`: "~/.config/graft/keys/it's",
		`c:\users\иван петров\x.yaml`:                  "~/x.yaml",
		`C:\Users\Иван Петрова\x`:                      "C:/Users/Иван Петрова/x",
		`D:\keys\ollama`:                               "D:/keys/ollama",
	} {
		if got := Tildify(in); got != want {
			t.Errorf("Tildify(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestResolvePathOnWindows: ~\ is the home directory as ~/ is.
func TestResolvePathOnWindows(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\u`)
	for in, want := range map[string]string{
		`~\keys\x`: `C:\Users\u\keys\x`,
		`~/keys/x`: `C:\Users\u\keys\x`,
		`~`:        `C:\Users\u`,
		`keys\x`:   `D:\cfg\keys\x`,
		`E:/k/x`:   `E:/k/x`, // an absolute path stays as written, as on Unix
	} {
		if got, err := ResolvePath(in, `D:\cfg`); err != nil || got != want {
			t.Errorf("ResolvePath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
