package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateState_CurrentVersion(t *testing.T) {
	s := NewState()
	s.Resources["web"] = &ResourceState{
		ID:   "container-123",
		Type: "docker_container",
		Attributes: map[string]interface{}{
			"image": "nginx",
		},
	}

	data, err := json.Marshal(s)
	require.NoError(t, err)

	migrated, err := MigrateState(data)
	require.NoError(t, err)

	assert.Equal(t, CurrentStateVersion, migrated.Version)
	assert.Len(t, migrated.Resources, 1)
	assert.Equal(t, "container-123", migrated.Resources["web"].ID)
}

func TestMigrateState_ZeroVersion(t *testing.T) {
	// Pre-versioned state (version 0 or missing) should be treated as version 1
	raw := `{"resources":{"test":{"id":"abc","type":"local_file","attributes":{"path":"/tmp/a"}}}}`

	migrated, err := MigrateState([]byte(raw))
	require.NoError(t, err)

	assert.Equal(t, CurrentStateVersion, migrated.Version)
	assert.Equal(t, "abc", migrated.Resources["test"].ID)
}

func TestMigrateState_FutureVersion(t *testing.T) {
	raw := `{"version":999,"resources":{}}`

	_, err := MigrateState([]byte(raw))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "newer than supported")
}

func TestMigrateState_InvalidJSON(t *testing.T) {
	_, err := MigrateState([]byte("not json"))
	assert.Error(t, err)
}
