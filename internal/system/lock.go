package system

import (
	"errors"
	"os"
	"syscall"
)

var ErrAnotherRunning = errors.New("another odin-up operation is currently running")

// Lock is a released advisory lock guarding installation operations.
type Lock struct {
	file *os.File
}

// AcquireLock takes an exclusive advisory lock on the given file. If another
// process already holds the lock, ErrAnotherRunning is returned.
func AcquireLock(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	_ = f.Chmod(0o644)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EACCES) {
			return nil, ErrAnotherRunning
		}
		return nil, err
	}
	return &Lock{file: f}, nil
}

// Release drops the lock.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}
