package storage

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// WriteJSONAtomic serializes before touching the destination, then publishes a
// complete replacement for game saves, shared stores and scoreboards.
func WriteJSONAtomic(path string, value any, perm os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(data, '\n'), perm)
}

// WriteFileAtomic writes, syncs and closes a unique temporary sibling before
// replacing path. Failures before replacement leave the previous file intact.
// Replacement uses the platform's rename operation; never delete the old file
// to retry a failed rename. Directory-entry durability on power loss depends on
// the filesystem and is not guaranteed by this API.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return writeFileAtomic(path, data, perm, func(dir, pattern string) (atomicFile, error) {
		return os.CreateTemp(dir, pattern)
	}, os.Rename)
}

type atomicFile interface {
	io.Writer
	Name() string
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

func writeFileAtomic(path string, data []byte, perm os.FileMode,
	create func(string, string) (atomicFile, error), replace func(string, string) error,
) error {
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := create(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		_ = os.Remove(f.Name())
	}()
	if err := f.Chmod(perm); err != nil {
		return err
	}
	n, err := f.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	closed = true
	if err != nil {
		return err
	}
	return replace(f.Name(), path)
}
