package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrConflict means that the file changed since the version a write was
// based on.
var ErrConflict = errors.New("the file changed on disk")

// Store reads and writes the config file. A version is the SHA-256 of the
// file in hex, "" when there is no file.
type Store struct {
	path string // as given; it may be a symbolic link

	// readFile is os.ReadFile, the read that Version makes after its stat; a
	// test puts in its place a read of a file that has just gone.
	readFile func(string) ([]byte, error)

	mu    sync.Mutex
	stamp stamp  // of the file the cached version is of
	cache string // its version
}

type stamp struct {
	size int64
	mod  time.Time
}

// NewStore returns the store of the config file at path.
func NewStore(path string) *Store {
	return &Store{path: path, readFile: os.ReadFile}
}

// Version returns the version of the file. It reads the file only when its
// size or modification time changed since the last call.
func (s *Store) Version() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Stat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if st := (stamp{info.Size(), info.ModTime()}); st != s.stamp || s.cache == "" {
		data, err := s.readFile(s.path)
		if errors.Is(err, fs.ErrNotExist) {
			// The file went after the stat, as when an editor saves by deleting
			// the file and creating it again: there is none, as for Read.
			return "", nil
		}
		if err != nil {
			return "", err
		}
		s.stamp, s.cache = st, version(data)
	}
	return s.cache, nil
}

// Read returns the file and its version; no file is nil and "".
func (s *Store) Read() ([]byte, string, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return data, version(data), nil
}

// Write replaces the file with data if the file is still at version base.
// It returns the new version, and the backup of the old file: <file>.bak,
// or "" when there was no file. When the config is a symbolic link, the
// file it points at is written, and the link stays. A new file is made with
// mode 0600 in a directory made with mode 0700; an old file keeps its mode.
func (s *Store) Write(base string, data []byte) (ver, backup string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, err := s.target()
	if err != nil {
		return "", "", err
	}
	old, err := os.ReadFile(target)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", "", err
	}
	if (exists && version(old) != base) || (!exists && base != "") {
		return "", "", ErrConflict
	}
	mode := fs.FileMode(0o600)
	if exists {
		info, err := os.Stat(target)
		if err != nil {
			return "", "", err
		}
		mode = info.Mode().Perm()
		backup = target + ".bak"
		if err := writeFile(backup, old, mode); err != nil {
			return "", "", err
		}
	} else if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", "", err
	}
	if err := writeFile(target, data, mode); err != nil {
		return "", "", err
	}
	s.stamp, s.cache = stamp{}, ""
	return version(data), backup, nil
}

// target returns the file the config path names, following symbolic links.
// A missing file is the path itself.
func (s *Store) target() (string, error) {
	t, err := filepath.EvalSymlinks(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		if _, lerr := os.Lstat(s.path); lerr == nil {
			return "", errors.New(s.path + " is a symbolic link to a missing file")
		}
		return s.path, nil
	}
	return t, err
}

// writeFile replaces path with data atomically: a temporary file in the
// same directory, synced, then renamed over path.
func writeFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // after the rename, there is nothing to remove
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

func version(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
