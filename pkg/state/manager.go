package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	StateDir          = ".goiac"
	StateFile         = "state.json"
	StateChecksumFile = "state.json.sha256"
	StateLockFile     = "state.lock"
)

type Manager struct {
	stateDir string
}

func NewManager() *Manager {
	return &Manager{
		stateDir: StateDir,
	}
}

// NewManagerWithDir creates a Manager using the given directory instead of the default.
// Intended for use in tests.
func NewManagerWithDir(dir string) *Manager {
	return &Manager{stateDir: dir}
}

func (m *Manager) Load() (*State, error) {
	statePath := filepath.Join(m.stateDir, StateFile)
	checksumPath := filepath.Join(m.stateDir, StateChecksumFile)

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Verify checksum if it exists
	if checksumData, err := os.ReadFile(checksumPath); err == nil {
		expected := string(checksumData)
		actual := computeChecksum(data)
		if expected != actual {
			return nil, fmt.Errorf("state file integrity check failed: checksum mismatch (expected %s, got %s)", expected, actual)
		}
	}

	// Migrate state to current version if needed
	migrated, err := MigrateState(data)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate state: %w", err)
	}
	return migrated, nil
}

func (m *Manager) Save(state *State) error {
	state.LastUpdated = time.Now().Format(time.RFC3339)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Ensure state directory exists
	if err := os.MkdirAll(m.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	statePath := filepath.Join(m.stateDir, StateFile)
	tmpPath := statePath + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	if err := os.Rename(tmpPath, statePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	// Write checksum file
	checksum := computeChecksum(data)
	checksumPath := filepath.Join(m.stateDir, StateChecksumFile)
	if err := os.WriteFile(checksumPath, []byte(checksum), 0644); err != nil {
		return fmt.Errorf("failed to write checksum file: %w", err)
	}

	return nil
}

func computeChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (m *Manager) Update(state *State, resourceID string, resourceState *ResourceState) {
	state.Resources[resourceID] = resourceState
}
func (m *Manager) DeleteResource(state *State, resourceID string) {
	delete(state.Resources, resourceID)
}
func (m *Manager) GetResource(state *State, resourceID string) (*ResourceState, bool) {
	res, ok := state.Resources[resourceID]
	return res, ok
}
