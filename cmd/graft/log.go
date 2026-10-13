package main

import (
	"errors"
	"os"
	"path/filepath"
)

// maxLog is the size past which a new session moves the log to graft.log.1.
const maxLog = 10 << 20

// logPath returns graft's log file: $XDG_STATE_HOME/graft/graft.log, else
// ~/.local/state/graft/graft.log.
func logPath(sys system) (string, error) {
	if dir, _ := sys.lookupEnv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "graft", "graft.log"), nil
	}
	if sys.home == "" {
		return "", errors.New("cannot find the home directory")
	}
	return filepath.Join(sys.home, ".local", "state", "graft", "graft.log"), nil
}

// openLog opens the log for appending, in a directory only the user can
// enter. A log larger than 10 MiB first moves to graft.log.1, over the one
// there.
func openLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil && info.Size() > maxLog {
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
}
