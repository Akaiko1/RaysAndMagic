package storage

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var errAtomicTest = errors.New("injected file failure")

type failingAtomicFile struct {
	*os.File
	stage string
}

func (f *failingAtomicFile) Chmod(mode os.FileMode) error {
	if f.stage == "chmod" {
		return errAtomicTest
	}
	return f.File.Chmod(mode)
}
func (f *failingAtomicFile) Write(data []byte) (int, error) {
	if f.stage == "write" || f.stage == "short_write" {
		n, err := f.File.Write(data[:2])
		if err != nil {
			return n, err
		}
		if f.stage == "write" {
			return n, errAtomicTest
		}
		return n, nil
	}
	return f.File.Write(data)
}
func (f *failingAtomicFile) Sync() error {
	if f.stage == "sync" {
		return errAtomicTest
	}
	return f.File.Sync()
}
func (f *failingAtomicFile) Close() error {
	err := f.File.Close()
	if f.stage == "close" {
		return errAtomicTest
	}
	return err
}

func TestAtomicReplacementFailureStages(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, stage := range []string{"create", "chmod", "write", "short_write", "sync", "close", "replace"} {
			t.Run(stage+map[bool]string{false: "/new", true: "/existing"}[existing], func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "save.json")
				previous := []byte("previous valid data")
				if existing {
					if err := os.WriteFile(path, previous, 0600); err != nil {
						t.Fatal(err)
					}
				}
				create := func(dir, pattern string) (atomicFile, error) {
					if stage == "create" {
						return nil, errAtomicTest
					}
					f, err := os.CreateTemp(dir, pattern)
					if err != nil {
						return nil, err
					}
					return &failingAtomicFile{f, stage}, nil
				}
				replace := func(from, to string) error {
					if stage == "replace" {
						return errAtomicTest
					}
					return os.Rename(from, to)
				}
				err := writeFileAtomic(path, []byte("replacement valid data"), 0644, create, replace)
				wantErr := errAtomicTest
				if stage == "short_write" {
					wantErr = io.ErrShortWrite
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("error=%v, want %v", err, wantErr)
				}
				got, err := os.ReadFile(path)
				if existing {
					if err != nil || !bytes.Equal(got, previous) {
						t.Fatalf("previous file changed: %q, %v", got, err)
					}
				} else if !os.IsNotExist(err) {
					t.Fatalf("failed creation published a file: %v", err)
				}
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				want := 0
				if existing {
					want = 1
				}
				if len(entries) != want {
					t.Fatalf("temporary file leaked: %v", entries)
				}
			})
		}
	}
}

func TestAtomicWritesPublishCompleteFiles(t *testing.T) {
	for _, jsonFile := range []bool{false, true} {
		t.Run(map[bool]string{false: "bytes", true: "json"}[jsonFile], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state")
			for _, value := range []string{"first", "replacement"} {
				var err error
				want := []byte(value)
				if jsonFile {
					err = WriteJSONAtomic(path, value, 0600)
					want = []byte("\"" + value + "\"\n")
				} else {
					err = WriteFileAtomic(path, want, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("published %q, want %q (%v)", got, want, err)
				}
			}
			if jsonFile {
				before, _ := os.ReadFile(path)
				if err := WriteJSONAtomic(path, math.NaN(), 0600); err == nil {
					t.Fatal("expected encoding failure")
				}
				after, _ := os.ReadFile(path)
				if !bytes.Equal(before, after) {
					t.Fatal("encoding failure changed file")
				}
			}
		})
	}
}

func TestAtomicConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	var wg sync.WaitGroup
	for _, value := range []byte{'a', 'b', 'c', 'd'} {
		wg.Add(1)
		go func(value byte) {
			defer wg.Done()
			if err := WriteFileAtomic(path, bytes.Repeat([]byte{value}, 16384), 0600); err != nil {
				t.Error(err)
			}
		}(value)
	}
	wg.Wait()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 16384 || !bytes.Equal(got, bytes.Repeat(got[:1], 16384)) {
		t.Fatal("mixed or incomplete publication")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("temporary siblings remain")
	}
}
