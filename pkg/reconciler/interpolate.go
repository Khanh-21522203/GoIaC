package reconciler

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/state"
	"fmt"
	"regexp"
)

// refPattern matches ${type.resource_id.attribute} interpolation expressions
var refPattern = regexp.MustCompile(`\$\{(\w+)\.(\w+)\.(\w+)\}`)

func InterpolateReferences(resource *config.Resource, currentState *state.State) *config.Resource {
	resolved := &config.Resource{
		ID:         resource.ID,
		Type:       resource.Type,
		Properties: interpolateProperties(resource.Properties, currentState),
	}
	return resolved
}

// interpolateProperties recursively interpolates all properties
func interpolateProperties(props map[string]interface{}, currentState *state.State) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range props {
		result[key] = interpolateValue(value, currentState)
	}

	return result
}

// interpolateValue interpolates a single value
func interpolateValue(value interface{}, currentState *state.State) interface{} {
	switch v := value.(type) {
	case string:
		return interpolateString(v, currentState)
	case map[string]interface{}:
		return interpolateProperties(v, currentState)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = interpolateValue(item, currentState)
		}
		return result
	default:
		return value
	}
}

// interpolateString replaces ${ref} patterns with actual values
func interpolateString(s string, currentState *state.State) string {
	return refPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := refPattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		// parts[1] = type (ignored)
		// parts[2] = resource ID
		// parts[3] = attribute

		resourceID := parts[2]
		attribute := parts[3]

		if resourceState, ok := currentState.Resources[resourceID]; ok {
			if attrValue, ok := resourceState.Attributes[attribute]; ok {
				return fmt.Sprint(attrValue)
			}
		}
		return match
	})
}
