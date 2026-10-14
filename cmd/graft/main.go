// Command graft starts Claude Code with other models: a profile names the
// models, the provider and the Claude Code settings of a session. See
// README.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"

	"github.com/zinin/graft/internal/config"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage:
  graft [--config FILE] [--dry-run] PROFILE [CLAUDE ARGS...]
  graft [--config FILE] list
  graft [--config FILE] check
  graft help | version

Starts Claude Code with the models and settings of PROFILE. The arguments
after PROFILE go to claude unchanged.

  --config FILE  the config file; by default $GRAFT_CONFIG, else
                 $XDG_CONFIG_HOME/graft/config.yaml, else ~/.config/graft/config.yaml
  --dry-run      print what graft would start, and start nothing
`

func main() {
	os.Exit(run(os.Args[1:], realSystem()))
}

// system is what run takes from the outside world; tests replace it.
type system struct {
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
	environ       []string
	home          string
	cwd           string
	lookPath      func(string) (string, error)
	exec          func(argv0 string, argv, envv []string) error
	claudeVersion func(path string) (string, error)
}

func realSystem() system {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	return system{
		stdin:         os.Stdin,
		stdout:        os.Stdout,
		stderr:        os.Stderr,
		environ:       os.Environ(),
		home:          home,
		cwd:           cwd,
		lookPath:      exec.LookPath,
		exec:          syscall.Exec,
		claudeVersion: claudeVersion,
	}
}

// lookupEnv finds name in the environment graft runs in.
func (s system) lookupEnv(name string) (string, bool) {
	for _, kv := range s.environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v, true
		}
	}
	return "", false
}

func run(args []string, sys system) int {
	flags := flag.NewFlagSet("graft", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configFlag := flags.String("config", "", "")
	dryRun := flags.Bool("dry-run", false, "")
	showVersion := flags.Bool("version", false, "")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(sys.stdout, usage)
			return 0
		}
		fmt.Fprintf(sys.stderr, "graft: %v\n%s", err, usage)
		return 1
	}
	rest := flags.Args()
	switch {
	case len(rest) > 1 && slices.Contains([]string{"help", "version", "list", "check"}, rest[0]):
		fmt.Fprintf(sys.stderr, "graft: %s takes no arguments\n", rest[0])
		return 1
	case *showVersion || (len(rest) > 0 && rest[0] == "version"):
		fmt.Fprintln(sys.stdout, "graft", buildVersion())
		return 0
	case len(rest) == 0:
		fmt.Fprint(sys.stderr, usage)
		return 1
	case rest[0] == "help":
		fmt.Fprint(sys.stdout, usage)
		return 0
	}
	path, err := configPath(*configFlag, sys)
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	cfg, err := config.Load(path)
	if err != nil {
		reportConfigError(sys.stderr, err)
		return 1
	}
	switch rest[0] {
	case "list":
		return list(cfg, sys)
	case "check":
		return check(cfg, sys)
	}
	return start(cfg, rest[0], rest[1:], *dryRun, sys)
}

// configPath picks the config file: --config, then $GRAFT_CONFIG, then
// $XDG_CONFIG_HOME/graft/config.yaml, then ~/.config/graft/config.yaml.
func configPath(flagValue string, sys system) (string, error) {
	path := flagValue
	if path == "" {
		path, _ = sys.lookupEnv("GRAFT_CONFIG")
	}
	if path == "" {
		if xdg, _ := sys.lookupEnv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "graft", "config.yaml"), nil
		}
		path = "~/.config/graft/config.yaml"
	}
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	if sys.home == "" {
		return "", errors.New("cannot find the home directory: set HOME or pass --config")
	}
	return filepath.Join(sys.home, path[1:]), nil
}

// reportConfigError prints every problem of the config, one per line.
func reportConfigError(w io.Writer, err error) {
	var errs config.Errors
	var pathErr *fs.PathError
	switch {
	case errors.As(err, &errs):
		for _, e := range errs {
			fmt.Fprintf(w, "graft: %v\n", e)
		}
	case errors.Is(err, fs.ErrNotExist) && errors.As(err, &pathErr):
		fmt.Fprintf(w, "graft: no config at %s: start from config.example.yaml in the graft repository\n",
			config.Tildify(pathErr.Path))
	default:
		fmt.Fprintf(w, "graft: %v\n", err)
	}
}

// buildVersion returns the version set at build time, or the module version
// that go install records.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
