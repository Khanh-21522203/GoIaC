package state

import (
	"encoding/json"
	"fmt"
)

// CurrentStateVersion is the latest state format version
const CurrentStateVersion = 1

// MigrateState takes raw JSON state data and migrates it to the current version.
// Returns the migrated State or an error if migration is not possible.
func MigrateState(data []byte) (*State, error) {
	// First, extract just the version field
	var versionProbe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &versionProbe); err != nil {
		return nil, fmt.Errorf("failed to read state version: %w", err)
	}

	version := versionProbe.Version
	if version == 0 {
		// Pre-versioned state: treat as version 1
		version = 1
	}

	if version > CurrentStateVersion {
		return nil, fmt.Errorf("state version %d is newer than supported version %d — please upgrade GoIaC", version, CurrentStateVersion)
	}

	// Apply migrations sequentially
	for version < CurrentStateVersion {
		var err error
		data, err = applyMigration(version, data)
		if err != nil {
			return nil, fmt.Errorf("failed to migrate state from version %d: %w", version, err)
		}
		version++
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal migrated state: %w", err)
	}

	state.Version = CurrentStateVersion
	return &state, nil
}

// applyMigration applies a single migration step from `fromVersion` to `fromVersion+1`.
// Add new migration cases here as the state format evolves.
func applyMigration(fromVersion int, data []byte) ([]byte, error) {
	switch fromVersion {
	// Example for future migrations:
	// case 1:
	//     return migrateV1toV2(data)
	default:
		return nil, fmt.Errorf("no migration path from version %d", fromVersion)
	}
}
