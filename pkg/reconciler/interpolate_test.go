package reconciler

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/state"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInterpolateReferences_Simple(t *testing.T) {
	currentState := state.NewState()
	currentState.Resources["net"] = &state.ResourceState{
		ID:   "network-abc",
		Type: "docker_network",
		Attributes: map[string]interface{}{
			"network_id": "network-abc",
			"name":       "app-net",
		},
	}

	resource := &config.Resource{
		ID:   "web",
		Type: "docker_container",
		Properties: map[string]interface{}{
			"image":      "nginx",
			"network_id": "${docker_network.net.network_id}",
		},
	}

	resolved := InterpolateReferences(resource, currentState)

	assert.Equal(t, "nginx", resolved.Properties["image"])
	assert.Equal(t, "network-abc", resolved.Properties["network_id"])
}

func TestInterpolateReferences_NoMatch(t *testing.T) {
	currentState := state.NewState()

	resource := &config.Resource{
		ID:   "web",
		Type: "docker_container",
		Properties: map[string]interface{}{
			"image":      "nginx",
			"network_id": "${docker_network.missing.id}",
		},
	}

	resolved := InterpolateReferences(resource, currentState)

	// Unresolved references should remain as-is
	assert.Equal(t, "${docker_network.missing.id}", resolved.Properties["network_id"])
}

func TestInterpolateReferences_NestedProperties(t *testing.T) {
	currentState := state.NewState()
	currentState.Resources["db"] = &state.ResourceState{
		ID:   "db-123",
		Type: "docker_container",
		Attributes: map[string]interface{}{
			"ip_address": "172.17.0.2",
		},
	}

	resource := &config.Resource{
		ID:   "app",
		Type: "docker_container",
		Properties: map[string]interface{}{
			"image": "myapp",
			"env": map[string]interface{}{
				"DB_HOST": "${docker_container.db.ip_address}",
			},
		},
	}

	resolved := InterpolateReferences(resource, currentState)

	env := resolved.Properties["env"].(map[string]interface{})
	assert.Equal(t, "172.17.0.2", env["DB_HOST"])
}

func TestInterpolateReferences_ListProperties(t *testing.T) {
	currentState := state.NewState()
	currentState.Resources["net"] = &state.ResourceState{
		ID:   "net-1",
		Type: "docker_network",
		Attributes: map[string]interface{}{
			"name": "my-net",
		},
	}

	resource := &config.Resource{
		ID:   "app",
		Type: "docker_container",
		Properties: map[string]interface{}{
			"image":    "myapp",
			"networks": []interface{}{"${docker_network.net.name}"},
		},
	}

	resolved := InterpolateReferences(resource, currentState)

	networks := resolved.Properties["networks"].([]interface{})
	assert.Equal(t, "my-net", networks[0])
}

func TestInterpolateReferences_DoesNotMutateOriginal(t *testing.T) {
	currentState := state.NewState()
	currentState.Resources["net"] = &state.ResourceState{
		ID:   "net-1",
		Type: "docker_network",
		Attributes: map[string]interface{}{
			"id": "net-1",
		},
	}

	resource := &config.Resource{
		ID:   "web",
		Type: "docker_container",
		Properties: map[string]interface{}{
			"network_id": "${docker_network.net.id}",
		},
	}

	resolved := InterpolateReferences(resource, currentState)

	// Original should be unchanged
	assert.Equal(t, "${docker_network.net.id}", resource.Properties["network_id"])
	// Resolved should have the value
	assert.Equal(t, "net-1", resolved.Properties["network_id"])
}

func TestInterpolateReferences_NonStringValues(t *testing.T) {
	currentState := state.NewState()

	resource := &config.Resource{
		ID:   "web",
		Type: "docker_container",
		Properties: map[string]interface{}{
			"image": "nginx",
			"port":  8080,
		},
	}

	resolved := InterpolateReferences(resource, currentState)

	assert.Equal(t, 8080, resolved.Properties["port"])
}
