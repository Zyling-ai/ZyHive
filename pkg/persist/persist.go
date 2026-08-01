// Package persist provides the shared durability primitives used by mutable
// on-disk state. Writers are serialized in-process and across processes.
package persist

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var pathLocks sync.Map

func mutexFor(path string) *sync.Mutex {
	clean := filepath.Clean(path)
	value, _ := pathLocks.LoadOrStore(clean, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// WithFileLock serializes fn for path in this process and with other ZyHive
// processes that use the same adjacent lock file.
func WithFileLock(path string, fn func() error) error {
	unlock, err := LockFile(path)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

// LockFile acquires the shared in-process and cross-process lock for path.
// The returned function must be called exactly once.
func LockFile(path string) (func() error, error) {
	mutex := mutexFor(path)
	mutex.Lock()

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		mutex.Unlock()
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	lock, err := acquireFileLock(path + ".lock")
	if err != nil {
		mutex.Unlock()
		return nil, err
	}
	var once sync.Once
	var unlockErr error
	return func() error {
		once.Do(func() {
			unlockErr = lock.Close()
			mutex.Unlock()
		})
		return unlockErr
	}, nil
}

// AtomicWrite replaces path only after the complete new content and its
// containing directory have been synchronized.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	cleanup = false
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("chmod replaced file: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("sync parent directory: %w", err)
	}
	return nil
}

// WriteFile commits one atomic replacement under the shared file lock.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	return WithFileLock(path, func() error {
		return AtomicWrite(path, data, mode)
	})
}
