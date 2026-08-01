//go:build windows

package persist

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

type fileLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func acquireFileLock(path string) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	lock := &fileLock{file: file}
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		&lock.overlapped,
	)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire file lock: %w", err)
	}
	return lock, nil
}

func (lock *fileLock) Close() error {
	unlockErr := windows.UnlockFileEx(
		windows.Handle(lock.file.Fd()),
		0,
		1,
		0,
		&lock.overlapped,
	)
	closeErr := lock.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func syncDir(string) error {
	// Windows rename durability is provided by the file flush and atomic
	// replacement; directory handles are not opened with backup semantics here.
	return nil
}
