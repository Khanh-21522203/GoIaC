package provider

import (
	"GoIaC/pkg/config"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateResource_ValidLocalFile(t *testing.T) {
	resource := &config.Resource{
		ID:   "test",
		Type: "local_file",
		Properties: map[string]interface{}{
			"path":    "/tmp/test.txt",
			"content": "hello",
		},
	}
	assert.NoError(t, ValidateResource(resource))
}

func TestValidateResource_ValidDockerContainer(t *testing.T) {
	resource := &config.Resource{
		ID:   "web",
		Type: "docker_container",
		Properties: map[string]interface{}{
			"image": "nginx",
			"port":  8080,
		},
	}
	assert.NoError(t, ValidateResource(resource))
}

func TestValidateResource_ValidDockerNetwork(t *testing.T) {
	resource := &config.Resource{
		ID:   "net",
		Type: "docker_network",
		Properties: map[string]interface{}{
			"name":   "app-net",
			"driver": "bridge",
		},
	}
	assert.NoError(t, ValidateResource(resource))
}

func TestValidateResource_MissingRequired(t *testing.T) {
	resource := &config.Resource{
		ID:         "web",
		Type:       "docker_container",
		Properties: map[string]interface{}{},
	}
	err := ValidateResource(resource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required property")
	assert.Contains(t, err.Error(), "image")
}

func TestValidateResource_UnknownProperty(t *testing.T) {
	resource := &config.Resource{
		ID:   "web",
		Type: "docker_container",
		Properties: map[string]interface{}{
			"image":   "nginx",
			"unknown": "value",
		},
	}
	err := ValidateResource(resource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown property")
}

func TestValidateResource_UnknownType(t *testing.T) {
	resource := &config.Resource{
		ID:         "x",
		Type:       "nonexistent_type",
		Properties: map[string]interface{}{},
	}
	err := ValidateResource(resource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown resource type")
}

func TestValidateResources_AllValid(t *testing.T) {
	resources := []*config.Resource{
		{ID: "f", Type: "local_file", Properties: map[string]interface{}{"path": "a", "content": "b"}},
		{ID: "c", Type: "docker_container", Properties: map[string]interface{}{"image": "nginx"}},
	}
	assert.NoError(t, ValidateResources(resources))
}

func TestValidateResources_OneInvalid(t *testing.T) {
	resources := []*config.Resource{
		{ID: "f", Type: "local_file", Properties: map[string]interface{}{"path": "a", "content": "b"}},
		{ID: "c", Type: "docker_container", Properties: map[string]interface{}{}}, // missing image
	}
	err := ValidateResources(resources)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required property")
}

func TestValidateResource_OptionalOnly(t *testing.T) {
	resource := &config.Resource{
		ID:   "web",
		Type: "docker_container",
		Properties: map[string]interface{}{
			"image": "nginx",
			// port and network_id are optional, not provided
		},
	}
	assert.NoError(t, ValidateResource(resource))
}
