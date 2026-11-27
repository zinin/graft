//go:build !windows

package ui

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/zinin/graft/internal/config"
)

// TestKeyFileNotRegular: a FIFO, a terminal or a device as a key file could
// hold a read until someone writes to it, or feed it without end, and the
// page checks the key after each edit: graft ui reads no such file.
func TestKeyFileNotRegular(t *testing.T) {
	s := startAPI(t, "")
	fifo := filepath.Join(filepath.Dir(s.path), "keys", "ollama")
	if err := os.MkdirAll(filepath.Dir(fifo), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	// Should a read wait on the FIFO all the same, a writer that comes and
	// goes ends it, so that the server can close.
	t.Cleanup(func() {
		if f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			f.Close()
		}
	})
	shown := config.Tildify(fifo)
	want := keyState{State: "error", Path: shown, Error: "file " + shown + ": not a regular file"}
	if st, err := s.checkKeyWithin(2*time.Second, "file", "keys/ollama", false); err != nil || st != want {
		t.Errorf("check: %+v %v, want %+v", st, err, want)
	}
	// A write that will not replace the file reads no key there either.
	var conflict keyConflict
	code, err := s.postWithin(2*time.Second, "/api/key/write", map[string]any{"provider": "ollama", "key": "sk-1"}, &conflict)
	if want := (keyConflict{Error: shown + " already exists"}); err != nil || code != 409 || conflict != want {
		t.Errorf("write: %d %+v %v, want 409 %+v", code, conflict, err, want)
	}
}
