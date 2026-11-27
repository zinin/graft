package launch

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Replace starts claude at argv0 with argv and envv in place of graft.
// Windows cannot replace a process, so claude runs as graft's child in
// graft's console, and graft exits with claude's exit code once claude ends.
// Meanwhile graft lets Ctrl+C, Ctrl+Break and the closing of the console
// pass: the console sends them to claude too, which handles them, and graft
// must not end before it. Replace returns only if claude does not start.
func Replace(argv0 string, argv, envv []string) error {
	cmd := exec.Command(argv0)
	cmd.Args = argv
	cmd.Env = envv
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Caught before claude starts, so that none finds graft unprepared.
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	if err := cmd.Start(); err != nil {
		signal.Stop(signals)
		return err
	}
	go func() {
		for range signals {
		}
	}()
	cmd.Wait()
	os.Exit(cmd.ProcessState.ExitCode())
	return nil
}
