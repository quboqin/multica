//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

type imageSlotLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func tryLockImageSlot(path string) (*imageSlotLock, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open image slot: %w", err)
	}
	lock := &imageSlotLock{file: file}
	err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &lock.overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		_ = file.Close()
		return nil, false, nil
	}
	if err != nil {
		_ = file.Close()
		return nil, false, fmt.Errorf("lock image slot: %w", err)
	}
	return lock, true, nil
}

func (l *imageSlotLock) release() error {
	unlockErr := windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &l.overlapped)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock image slot: %w", unlockErr)
	}
	return closeErr
}
