package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
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

	// duringWrite, nil but in a test, runs in Write once the new text is in
	// its temporary file, right before the file is read again: there a test
	// changes the file, as an editor saving at that moment would.
	duringWrite func()

	mu sync.Mutex
	// info is the stat of the file the cached version is of, nil when none
	// is cached. The version holds while the file at path is the same file
	// (the same device and inode) with the same size and modification time;
	// for another file, or a change of either, Version reads the file again.
	info  fs.FileInfo
	cache string // its version
}

// NewStore returns the store of the config file at path.
func NewStore(path string) *Store {
	return &Store{path: path, readFile: os.ReadFile}
}

// Version returns the version of the file. It reads the file only when it
// is another file (another device or inode), or its size or modification
// time changed, since the last call.
func (s *Store) Version() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := os.Stat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		// A file created again may get the inode, size and modification time
		// of the one that went: its version must be read, not remembered.
		s.info, s.cache = nil, ""
		return "", nil
	}
	if err != nil {
		return "", err
	}
	same := s.info != nil && os.SameFile(s.info, cur) && s.info.Size() == cur.Size() && s.info.ModTime().Equal(cur.ModTime())
	if !same {
		data, err := s.readFile(s.path)
		if errors.Is(err, fs.ErrNotExist) {
			// The file went after the stat, as when an editor saves by deleting
			// the file and creating it again: there is none, as for Read.
			s.info, s.cache = nil, ""
			return "", nil
		}
		if err != nil {
			return "", err
		}
		s.info, s.cache = cur, version(data)
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
// The file is checked again right before it is replaced, so that a write
// that came during the save, from an editor or another graft ui, is not
// lost without a word.
func (s *Store) Write(base string, data []byte) (ver, backup string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, err := s.target()
	if err != nil {
		return "", "", err
	}
	old, exists, err := readUnchanged(target, base)
	if err != nil {
		return "", "", err
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
	tmp, err := writeTemp(target, data, mode)
	if err != nil {
		return "", "", err
	}
	defer os.Remove(tmp) // after the rename, there is nothing to remove
	if s.duringWrite != nil {
		s.duringWrite()
	}
	// The backup and the temporary file take two syncs, time enough for an
	// editor to save the file, so it is read once more. There is no lock
	// across processes: an editor takes none, and a second graft ui on the
	// same file is not what the server is made for, as a second tab opens
	// the same address. Only a moment is left between this read and the
	// rename. After a refusal here, the backup made above holds the file as
	// the first check read it.
	if _, _, err := readUnchanged(target, base); err != nil {
		return "", "", err
	}
	if err := replace(tmp, target); err != nil {
		return "", "", err
	}
	s.info, s.cache = nil, ""
	return version(data), backup, nil
}

// readUnchanged returns the file at path, and whether there is one, if it
// is still at version base: ErrConflict if it is not.
func readUnchanged(path, base string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, false, err
	}
	if (exists && version(data) != base) || (!exists && base != "") {
		return nil, false, ErrConflict
	}
	return data, exists, nil
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
	tmp, err := writeTemp(path, data, mode)
	if err != nil {
		return err
	}
	defer os.Remove(tmp) // after the rename, there is nothing to remove
	return replace(tmp, path)
}

// writeTemp writes data with mode to a new temporary file in the directory
// of path, synced, for replace to rename over path. It returns the name of
// the file; after an error, there is none.
func writeTemp(path string, data []byte, mode fs.FileMode) (tmp string, err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			os.Remove(f.Name())
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// replace renames tmp over path, then syncs the directory, so that the
// rename survives a crash.
func replace(tmp, path string) error {
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

func version(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
