//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type imageSlotLock struct {
	file *os.File
}

func tryLockImageSlot(path string) (*imageSlotLock, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open image slot: %w", err)
	}
	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		_ = file.Close()
		return nil, false, nil
	}
	if err != nil {
		_ = file.Close()
		return nil, false, fmt.Errorf("lock image slot: %w", err)
	}
	return &imageSlotLock{file: file}, true, nil
}

func (l *imageSlotLock) release() error {
	unlockErr := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock image slot: %w", unlockErr)
	}
	return closeErr
}
