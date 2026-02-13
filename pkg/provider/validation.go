package provider

import (
	"GoIaC/pkg/config"
	"fmt"
)

// PropertySchema defines the expected properties for a resource type
type PropertySchema struct {
	Required []string
	Optional []string
}

// knownSchemas maps resource types to their property schemas
var knownSchemas = map[string]PropertySchema{
	"local_file": {
		Required: []string{"path", "content"},
		Optional: []string{},
	},
	"docker_container": {
		Required: []string{"image"},
		Optional: []string{"port", "network_id"},
	},
	"docker_network": {
		Required: []string{"name"},
		Optional: []string{"driver"},
	},
}

// ValidateResource checks that a resource has valid properties for its type
func ValidateResource(resource *config.Resource) error {
	schema, ok := knownSchemas[resource.Type]
	if !ok {
		return fmt.Errorf("unknown resource type: %s", resource.Type)
	}

	// Check required properties
	for _, req := range schema.Required {
		if _, exists := resource.Properties[req]; !exists {
			return fmt.Errorf("resource %s (%s): missing required property %q", resource.ID, resource.Type, req)
		}
	}

	// Check for unknown properties
	allowed := make(map[string]bool)
	for _, p := range schema.Required {
		allowed[p] = true
	}
	for _, p := range schema.Optional {
		allowed[p] = true
	}

	for key := range resource.Properties {
		if !allowed[key] {
			return fmt.Errorf("resource %s (%s): unknown property %q", resource.ID, resource.Type, key)
		}
	}

	return nil
}

// ValidateResources validates all resources in a config
func ValidateResources(resources []*config.Resource) error {
	for _, resource := range resources {
		if err := ValidateResource(resource); err != nil {
			return err
		}
	}
	return nil
}
