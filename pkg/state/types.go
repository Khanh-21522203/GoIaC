package state

import "time"

// State represents the actual infrastructure state
type State struct {
	Version     int                       `json:"version"`
	LastUpdated string                    `json:"last_updated"`
	Resources   map[string]*ResourceState `json:"resources"`
}

type ResourceState struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Attributes map[string]interface{} `json:"attributes"`
}

type LockInfo struct {
	LockedAt  time.Time `json:"locked_at"`
	LockedBy  string    `json:"locked_by"`
	ProcessID int       `json:"process_id"`
}

func NewState() *State {
	return &State{
		Version:   1,
		Resources: make(map[string]*ResourceState),
	}
}
