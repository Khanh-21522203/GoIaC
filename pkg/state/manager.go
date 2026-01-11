package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	StateDir      = ".goiac"
	StateFile     = "state.json"
	StateLockFile = "state.lock"
)

type Manager struct {
	stateDir string
}

func NewManager() *Manager {
	return &Manager{
		stateDir: StateDir,
	}
}

func (m *Manager) Load() (*State, error) {
	statePath := filepath.Join(m.stateDir, StateFile)

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	return &state, nil
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

	return nil
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
