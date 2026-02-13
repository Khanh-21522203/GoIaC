package graph

import (
	"GoIaC/pkg/config"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraph_Build(t *testing.T) {
	resources := []*config.Resource{
		{ID: "net", Type: "docker_network", Properties: map[string]interface{}{"name": "app-net"}},
		{ID: "web", Type: "docker_container", Properties: map[string]interface{}{
			"image":      "nginx",
			"network_id": "${docker_network.net.id}",
		}},
	}

	g := NewGraph()
	err := g.Build(resources)
	require.NoError(t, err)

	nodes := g.GetNodes()
	assert.Len(t, nodes, 2)
	assert.Contains(t, nodes, "net")
	assert.Contains(t, nodes, "web")

	// web depends on net
	deps := g.GetDependencies("web")
	assert.Contains(t, deps, "net")

	// net has no dependencies
	deps = g.GetDependencies("net")
	assert.Empty(t, deps)
}

func TestGraph_ValidateDAG_NoCycle(t *testing.T) {
	resources := []*config.Resource{
		{ID: "a", Type: "local_file", Properties: map[string]interface{}{"path": "a.txt", "content": "a"}},
		{ID: "b", Type: "local_file", Properties: map[string]interface{}{"path": "b.txt", "content": "${local_file.a.path}"}},
	}

	g := NewGraph()
	require.NoError(t, g.Build(resources))
	assert.NoError(t, g.ValidateDAG())
}

func TestGraph_ValidateDAG_WithCycle(t *testing.T) {
	g := NewGraph()
	g.nodes["a"] = &Node{ID: "a", Type: "t"}
	g.nodes["b"] = &Node{ID: "b", Type: "t"}
	g.edges["a"] = []string{"b"}
	g.edges["b"] = []string{"a"}

	err := g.ValidateDAG()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestGraph_ValidateReferences_Valid(t *testing.T) {
	resources := []*config.Resource{
		{ID: "net", Type: "docker_network", Properties: map[string]interface{}{"name": "n"}},
		{ID: "web", Type: "docker_container", Properties: map[string]interface{}{
			"image":      "nginx",
			"network_id": "${docker_network.net.id}",
		}},
	}

	g := NewGraph()
	require.NoError(t, g.Build(resources))
	assert.NoError(t, g.ValidateReferences())
}

func TestGraph_ValidateReferences_Invalid(t *testing.T) {
	resources := []*config.Resource{
		{ID: "web", Type: "docker_container", Properties: map[string]interface{}{
			"image":      "nginx",
			"network_id": "${docker_network.missing.id}",
		}},
	}

	g := NewGraph()
	require.NoError(t, g.Build(resources))
	err := g.ValidateReferences()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "undefined resource")
}

func TestGraph_TopologicalSort(t *testing.T) {
	resources := []*config.Resource{
		{ID: "net", Type: "docker_network", Properties: map[string]interface{}{"name": "n"}},
		{ID: "web", Type: "docker_container", Properties: map[string]interface{}{
			"image":      "nginx",
			"network_id": "${docker_network.net.id}",
		}},
	}

	g := NewGraph()
	require.NoError(t, g.Build(resources))

	order, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Len(t, order, 2)

	// edges: web → net (web depends on net)
	// Kahn's processes nodes with inDegree 0 first.
	// web has inDegree 0, net has inDegree 1 → web comes first.
	webIdx := -1
	netIdx := -1
	for i, id := range order {
		if id == "web" {
			webIdx = i
		}
		if id == "net" {
			netIdx = i
		}
	}
	assert.True(t, webIdx < netIdx, "web (dependent) should come before net (dependency) in topological order")
}

func TestGraph_TopologicalSortReverse(t *testing.T) {
	resources := []*config.Resource{
		{ID: "net", Type: "docker_network", Properties: map[string]interface{}{"name": "n"}},
		{ID: "web", Type: "docker_container", Properties: map[string]interface{}{
			"image":      "nginx",
			"network_id": "${docker_network.net.id}",
		}},
	}

	g := NewGraph()
	require.NoError(t, g.Build(resources))

	order, err := g.TopologicalSortReverse()
	require.NoError(t, err)
	assert.Len(t, order, 2)

	// Reverse of [web, net] is [net, web]
	// In reverse order, net (dependency) comes before web (dependent)
	netIdx := -1
	webIdx := -1
	for i, id := range order {
		if id == "net" {
			netIdx = i
		}
		if id == "web" {
			webIdx = i
		}
	}
	assert.True(t, netIdx < webIdx, "net (dependency) should come before web (dependent) in reverse topological order")
}

func TestExtractReferences(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]interface{}
		expected   []string
	}{
		{
			name:       "no references",
			properties: map[string]interface{}{"image": "nginx"},
			expected:   []string{},
		},
		{
			name:       "single reference",
			properties: map[string]interface{}{"network_id": "${docker_network.net.id}"},
			expected:   []string{"net"},
		},
		{
			name: "multiple references",
			properties: map[string]interface{}{
				"network_id": "${docker_network.net.id}",
				"volume":     "${local_file.data.path}",
			},
			expected: []string{"net", "data"},
		},
		{
			name: "nested reference",
			properties: map[string]interface{}{
				"config": map[string]interface{}{
					"upstream": "${docker_container.api.ip_address}",
				},
			},
			expected: []string{"api"},
		},
		{
			name: "duplicate references deduplicated",
			properties: map[string]interface{}{
				"a": "${docker_network.net.id}",
				"b": "${docker_network.net.name}",
			},
			expected: []string{"net"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := ExtractReferences(tt.properties)
			assert.ElementsMatch(t, tt.expected, refs)
		})
	}
}

func TestGraph_NoDependencies(t *testing.T) {
	resources := []*config.Resource{
		{ID: "a", Type: "local_file", Properties: map[string]interface{}{"path": "a.txt", "content": "a"}},
		{ID: "b", Type: "local_file", Properties: map[string]interface{}{"path": "b.txt", "content": "b"}},
	}

	g := NewGraph()
	require.NoError(t, g.Build(resources))
	assert.NoError(t, g.ValidateDAG())

	order, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Len(t, order, 2)
}
