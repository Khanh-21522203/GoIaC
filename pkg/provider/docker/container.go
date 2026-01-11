package docker

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/state"
	"context"
	"fmt"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type ContainerProvider struct {
	client *client.Client
}

func NewContainerProvider() (*ContainerProvider, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &ContainerProvider{client: cli}, nil
}

func (p *ContainerProvider) Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error) {
	image, ok := desired.Properties["image"].(string)
	if !ok {
		return nil, fmt.Errorf("image property required")
	}

	// Handle YAML number type (float64)
	var port int
	if portVal, ok := desired.Properties["port"]; ok {
		switch v := portVal.(type) {
		case float64:
			port = int(v)
		case int:
			port = v
		default:
			return nil, fmt.Errorf("port must be a number")
		}
	}
	// Optional network_id
	networkID, _ := desired.Properties["network_id"].(string)

	containerConfig := &container.Config{
		Image: image,
	}
	hostConfig := &container.HostConfig{}

	if port > 0 {
		portStr := fmt.Sprintf("%d/tcp", port)
		containerConfig.ExposedPorts = nat.PortSet{
			nat.Port(portStr): struct{}{},
		}
		hostConfig.PortBindings = nat.PortMap{
			nat.Port(portStr): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: strconv.Itoa(port),
				},
			},
		}
	}

	if networkID != "" {
		hostConfig.NetworkMode = container.NetworkMode(networkID)
	}

	resp, err := p.client.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		nil,
		nil,
		desired.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := p.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	attributes := map[string]interface{}{
		"container_id": resp.ID,
		"image":        image,
		"status":       "running",
	}

	if port > 0 {
		attributes["port"] = port
	}
	if networkID != "" {
		attributes["network_id"] = networkID
	}

	return &state.ResourceState{
		ID:         resp.ID,
		Type:       desired.Type,
		Attributes: attributes,
	}, nil
}

func (p *ContainerProvider) Read(ctx context.Context, resourceID string) (*state.ResourceState, error) {
	inspect, err := p.client.ContainerInspect(ctx, resourceID)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	attributes := map[string]interface{}{
		"container_id": inspect.ID,
		"image":        inspect.Config.Image,
		"status":       inspect.State.Status,
	}

	if inspect.NetworkSettings.IPAddress != "" {
		attributes["ip_address"] = inspect.NetworkSettings.IPAddress
	}

	return &state.ResourceState{
		ID:         inspect.ID,
		Type:       "docker_container",
		Attributes: attributes,
	}, nil
}

func (p *ContainerProvider) Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error) {
	if err := p.Delete(ctx, resourceID); err != nil {
		return nil, err
	}

	return p.Create(ctx, desired)
}

func (p *ContainerProvider) Delete(ctx context.Context, resourceID string) error {
	timeout := 10
	err := p.client.ContainerStop(ctx, resourceID, container.StopOptions{
		Timeout: &timeout,
	})
	if err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	err = p.client.ContainerRemove(ctx, resourceID, container.RemoveOptions{
		Force: true,
	})
	if err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	return nil
}
