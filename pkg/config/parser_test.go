package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Parse(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "main.yaml")

	configContent := `
resources:
  - id: web
    type: docker_container
    properties:
      image: nginx
      port: 8080
  - id: net
    type: docker_network
    properties:
      name: app-network
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Parse
	parser := NewParser()
	resources, err := parser.Parse(configPath)
	require.NoError(t, err)

	// Verify
	assert.Len(t, resources, 2)
	assert.Equal(t, "web", resources[0].ID)
	assert.Equal(t, "docker_container", resources[0].Type)
	assert.Equal(t, "nginx", resources[0].Properties["image"])
	assert.Equal(t, 8080, resources[0].Properties["port"]) // YAML numbers are int
}

func TestParser_ParseString(t *testing.T) {
	yamlStr := `
resources:
  - id: test
    type: docker_container
    properties:
      image: nginx
`

	parser := NewParser()
	resources, err := parser.ParseString(yamlStr)
	require.NoError(t, err)

	assert.Len(t, resources, 1)
	assert.Equal(t, "test", resources[0].ID)
}

func TestParser_ValidateDuplicateIDs(t *testing.T) {
	yamlStr := `
resources:
  - id: web
    type: docker_container
    properties: {}
  - id: web
    type: docker_network
    properties: {}
`

	parser := NewParser()
	_, err := parser.ParseString(yamlStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate resource")
}

func TestParser_ValidateMissingID(t *testing.T) {
	yamlStr := `
resources:
  - type: docker_container
    properties: {}
`

	parser := NewParser()
	_, err := parser.ParseString(yamlStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing ID")
}

func TestParser_ValidateMissingType(t *testing.T) {
	yamlStr := `
resources:
  - id: web
    properties: {}
`

	parser := NewParser()
	_, err := parser.ParseString(yamlStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing type")
}

func TestParser_EmptyConfig(t *testing.T) {
	yamlStr := `resources: []`

	parser := NewParser()
	_, err := parser.ParseString(yamlStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no resources defined")
}
