package reconciler

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/state"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeDiff_AllCreate(t *testing.T) {
	desired := []*config.Resource{
		{ID: "web", Type: "docker_container", Properties: map[string]interface{}{"image": "nginx"}},
		{ID: "net", Type: "docker_network", Properties: map[string]interface{}{"name": "app-net"}},
	}
	currentState := state.NewState()

	changes := ComputeDiff(desired, currentState)

	assert.Len(t, changes, 2)
	for _, c := range changes {
		assert.Equal(t, ChangeTypeCreate, c.Type)
	}
}

func TestComputeDiff_AllNoop(t *testing.T) {
	desired := []*config.Resource{
		{ID: "web", Type: "docker_container", Properties: map[string]interface{}{"image": "nginx"}},
	}
	currentState := state.NewState()
	currentState.Resources["web"] = &state.ResourceState{
		ID:   "container-123",
		Type: "docker_container",
		Attributes: map[string]interface{}{
			"image": "nginx",
		},
	}

	changes := ComputeDiff(desired, currentState)

	assert.Len(t, changes, 1)
	assert.Equal(t, ChangeTypeNoop, changes[0].Type)
}

func TestComputeDiff_Update(t *testing.T) {
	desired := []*config.Resource{
		{ID: "web", Type: "docker_container", Properties: map[string]interface{}{"image": "nginx:latest"}},
	}
	currentState := state.NewState()
	currentState.Resources["web"] = &state.ResourceState{
		ID:   "container-123",
		Type: "docker_container",
		Attributes: map[string]interface{}{
			"image": "nginx:1.25",
		},
	}

	changes := ComputeDiff(desired, currentState)

	assert.Len(t, changes, 1)
	assert.Equal(t, ChangeTypeUpdate, changes[0].Type)
	assert.Contains(t, changes[0].Reason, "image changed")
}

func TestComputeDiff_Delete(t *testing.T) {
	desired := []*config.Resource{} // empty desired
	currentState := state.NewState()
	currentState.Resources["old"] = &state.ResourceState{
		ID:   "container-old",
		Type: "docker_container",
		Attributes: map[string]interface{}{
			"image": "nginx",
		},
	}

	changes := ComputeDiff(desired, currentState)

	assert.Len(t, changes, 1)
	assert.Equal(t, ChangeTypeDelete, changes[0].Type)
	assert.NotNil(t, changes[0].OldState)
}

func TestComputeDiff_Mixed(t *testing.T) {
	desired := []*config.Resource{
		{ID: "keep", Type: "local_file", Properties: map[string]interface{}{"path": "/tmp/a", "content": "hello"}},
		{ID: "new", Type: "local_file", Properties: map[string]interface{}{"path": "/tmp/b", "content": "world"}},
	}
	currentState := state.NewState()
	currentState.Resources["keep"] = &state.ResourceState{
		ID:   "/tmp/a",
		Type: "local_file",
		Attributes: map[string]interface{}{
			"path":    "/tmp/a",
			"content": "hello",
		},
	}
	currentState.Resources["remove"] = &state.ResourceState{
		ID:   "/tmp/c",
		Type: "local_file",
		Attributes: map[string]interface{}{
			"path":    "/tmp/c",
			"content": "old",
		},
	}

	changes := ComputeDiff(desired, currentState)

	typeCount := map[ChangeType]int{}
	for _, c := range changes {
		typeCount[c.Type]++
	}

	assert.Equal(t, 1, typeCount[ChangeTypeNoop], "keep should be noop")
	assert.Equal(t, 1, typeCount[ChangeTypeCreate], "new should be created")
	assert.Equal(t, 1, typeCount[ChangeTypeDelete], "remove should be deleted")
}

func TestPropertiesDiffer_NumericTypes(t *testing.T) {
	// YAML returns int, state may have float64 after JSON round-trip
	desired := map[string]interface{}{"port": 8080}
	actual := map[string]interface{}{"port": float64(8080)}

	assert.False(t, propertiesDiffer(desired, actual), "int 8080 and float64 8080 should be equal after normalization")
}

func TestPropertiesDiffer_NestedMaps(t *testing.T) {
	desired := map[string]interface{}{
		"config": map[string]interface{}{"key": "value"},
	}
	actual := map[string]interface{}{
		"config": map[string]interface{}{"key": "value"},
	}

	assert.False(t, propertiesDiffer(desired, actual))
}

func TestPropertiesDiffer_MissingKey(t *testing.T) {
	desired := map[string]interface{}{"image": "nginx", "port": 80}
	actual := map[string]interface{}{"image": "nginx"}

	assert.True(t, propertiesDiffer(desired, actual))
}
