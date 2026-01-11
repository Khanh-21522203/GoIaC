package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateManager_SaveAndLoad(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()
	manager := &Manager{stateDir: tmpDir}

	// Create state
	state := NewState()
	state.Resources["web"] = &ResourceState{
		ID:   "container-123",
		Type: "docker_container",
		Attributes: map[string]interface{}{
			"image": "nginx",
			"port":  8080,
		},
	}

	// Save state
	err := manager.Save(state)
	require.NoError(t, err)

	// Verify file exists
	statePath := filepath.Join(tmpDir, StateFile)
	_, err = os.Stat(statePath)
	require.NoError(t, err)

	// Load state
	loaded, err := manager.Load()
	require.NoError(t, err)

	// Verify content
	assert.Equal(t, 1, loaded.Version)
	assert.Len(t, loaded.Resources, 1)
	assert.Equal(t, "container-123", loaded.Resources["web"].ID)
	assert.Equal(t, "nginx", loaded.Resources["web"].Attributes["image"])
}

func TestStateManager_LoadEmpty(t *testing.T) {
	// Setup temp directory (no state file)
	tmpDir := t.TempDir()
	manager := &Manager{stateDir: tmpDir}

	// Load should return empty state
	state, err := manager.Load()
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, 1, state.Version)
	assert.Empty(t, state.Resources)
}

func TestStateManager_Lock(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &Manager{stateDir: tmpDir}

	// First lock should succeed
	err := manager.Lock()
	require.NoError(t, err)

	// Second lock should fail
	err = manager.Lock()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "state locked")

	// Unlock
	err = manager.Unlock()
	require.NoError(t, err)

	// Lock again should succeed
	err = manager.Lock()
	require.NoError(t, err)

	// Cleanup
	manager.Unlock()
}

func TestStateManager_WithLock(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &Manager{stateDir: tmpDir}

	executed := false
	err := manager.WithLock(func() error {
		executed = true

		// Verify lock exists
		lockPath := filepath.Join(tmpDir, StateLockFile)
		_, err := os.Stat(lockPath)
		assert.NoError(t, err)

		return nil
	})

	require.NoError(t, err)
	assert.True(t, executed)

	// Verify lock is released
	lockPath := filepath.Join(tmpDir, StateLockFile)
	_, err = os.Stat(lockPath)
	assert.True(t, os.IsNotExist(err))
}
