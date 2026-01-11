package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const staleLockDuration = 30 * time.Minute
const LockMaxRetries = 3

var (
	ErrRetryLock = errors.New("retry lock")
)

func (m *Manager) Lock() error {
	lockPath := filepath.Join(m.stateDir, StateLockFile)

	if err := os.MkdirAll(m.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	for i := 0; i < LockMaxRetries; i++ {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			if os.IsExist(err) {
				err2 := m.checkStaleLock(lockPath)
				if errors.Is(err2, ErrRetryLock) {
					continue
				}
				return err2
			}
			return fmt.Errorf("failed to create lock file: %w", err)
		}
		defer f.Close()

		lockInfo := LockInfo{
			LockedAt:  time.Now(),
			LockedBy:  "goiac",
			ProcessID: os.Getpid(),
		}

		if err := json.NewEncoder(f).Encode(lockInfo); err != nil {
			_ = os.Remove(lockPath)
			return fmt.Errorf("failed to write lock info: %w", err)
		}
		return nil
	}

	return fmt.Errorf("failed to acquire lock due to contention, please retry")
}

func (m *Manager) Unlock() error {
	lockPath := filepath.Join(m.stateDir, StateLockFile)

	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}
	return nil
}

func (m *Manager) checkStaleLock(lockPath string) error {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return fmt.Errorf("failed to read lock file: %w", err)
	}

	var lockInfo LockInfo
	if err := json.Unmarshal(data, &lockInfo); err != nil {
		_ = os.Remove(lockPath)
		return ErrRetryLock
	}

	if time.Since(lockInfo.LockedAt) > staleLockDuration {
		_ = os.Remove(lockPath)
		return ErrRetryLock
	}

	return fmt.Errorf("state locked by process %d at %s (acquired %s ago)",
		lockInfo.ProcessID,
		lockInfo.LockedAt.Format(time.RFC3339),
		time.Since(lockInfo.LockedAt).Round(time.Second))
}

func (m *Manager) WithLock(fn func() error) error {
	if err := m.Lock(); err != nil {
		return err
	}
	defer m.Unlock()

	return fn()
}
