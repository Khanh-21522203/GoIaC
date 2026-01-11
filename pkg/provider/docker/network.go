package docker

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/state"
	"context"
	"fmt"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type NetworkProvider struct {
	client *client.Client
}

func NewNetworkProvider() (*NetworkProvider, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &NetworkProvider{client: cli}, nil
}

func (p *NetworkProvider) Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error) {
	name, ok := desired.Properties["name"].(string)
	if !ok {
		return nil, fmt.Errorf("name property required")
	}

	driver := "bridge" // Default driver
	if driveVal, ok := desired.Properties["driver"].(string); ok {
		driver = driveVal
	}

	// Create network
	resp, err := p.client.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: driver,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}

	return &state.ResourceState{
		ID:   resp.ID,
		Type: desired.Type,
		Attributes: map[string]interface{}{
			"network_id": resp.ID,
			"name":       name,
			"driver":     driver,
		},
	}, nil
}

func (p *NetworkProvider) Read(ctx context.Context, resourceID string) (*state.ResourceState, error) {
	inspect, err := p.client.NetworkInspect(ctx, resourceID, network.InspectOptions{})
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to inspect network: %w", err)
	}

	return &state.ResourceState{
		ID:   inspect.ID,
		Type: "docker_netwotk",
		Attributes: map[string]interface{}{
			"network_id": inspect.ID,
			"name":       inspect.Name,
			"driver":     inspect.Driver,
		},
	}, nil
}

func (p *NetworkProvider) Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error) {
	// For networks, update = recreate
	if err := p.Delete(ctx, resourceID); err != nil {
		return nil, err
	}

	return p.Create(ctx, desired)
}

func (p *NetworkProvider) Delete(ctx context.Context, resourceID string) error {
	err := p.client.NetworkRemove(ctx, resourceID)
	if err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("failed to remove network: %w", err)
	}
	return nil
}
