package provider

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/state"
	"context"
)

type Provider interface {
	// Create creates a new resource
	// Returns the resource state with ID and attributes
	Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)

	// Read retrieves the current state of a resource
	// Returns (nil, nil) if resource doesn't exist (NOT an error)
	Read(ctx context.Context, resourceID string) (*state.ResourceState, error)

	// Update modifies an existing resource
	// Returns the updated resource state
	Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error)

	// Delete removes a resource
	// Must be idempotent (safe to call multiple times)
	Delete(ctx context.Context, resourceID string) error
}
