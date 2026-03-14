package reconciler

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/state"
	"fmt"
	"reflect"
)

type ChangeType int

const (
	ChangeTypeCreate ChangeType = iota
	ChangeTypeUpdate
	ChangeTypeDelete
	ChangeTypeNoop
)

type Change struct {
	Type             ChangeType
	Resource         *config.Resource
	OldState         *state.ResourceState
	Reason           string
	ConfigResourceID string // config-level resource ID (map key in state)
}

func ComputeDiff(desired []*config.Resource, actual *state.State) []*Change {
	var changes []*Change
	processed := make(map[string]bool)

	for _, resource := range desired {
		processed[resource.ID] = true

		oldState, exists := actual.Resources[resource.ID]
		if !exists {
			changes = append(changes, &Change{
				Type:             ChangeTypeCreate,
				Resource:         resource,
				Reason:           "resource does not exist",
				ConfigResourceID: resource.ID,
			})
		} else if propertiesDiffer(resource.Properties, oldState.Attributes) {
			changes = append(changes, &Change{
				Type:             ChangeTypeUpdate,
				Resource:         resource,
				OldState:         oldState,
				Reason:           computeChangedFields(resource.Properties, oldState.Attributes),
				ConfigResourceID: resource.ID,
			})
		} else {
			changes = append(changes, &Change{
				Type:             ChangeTypeNoop,
				Resource:         resource,
				OldState:         oldState,
				ConfigResourceID: resource.ID,
			})
		}
	}

	// Check for resources to delete (in actual but not desired)
	for resourceID, oldState := range actual.Resources {
		if !processed[resourceID] {
			changes = append(changes, &Change{
				Type:             ChangeTypeDelete,
				OldState:         oldState,
				Reason:           "resource no longer in configuration",
				ConfigResourceID: resourceID,
			})
		}
	}

	return changes
}

// propertiesDiffer checks if properties differ from attributes
func propertiesDiffer(desired map[string]interface{}, actual map[string]interface{}) bool {
	for key, desiredValue := range desired {
		actualValue, exists := actual[key]
		if !exists || !reflect.DeepEqual(normalizeValue(desiredValue), normalizeValue(actualValue)) {
			return true
		}
	}
	return false
}

// normalizeValue converts numeric types to float64 for consistent comparison
// (YAML/JSON unmarshal may produce different numeric types)
func normalizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float32:
		return float64(val)
	case map[string]interface{}:
		normalized := make(map[string]interface{}, len(val))
		for k, inner := range val {
			normalized[k] = normalizeValue(inner)
		}
		return normalized
	case []interface{}:
		normalized := make([]interface{}, len(val))
		for i, inner := range val {
			normalized[i] = normalizeValue(inner)
		}
		return normalized
	default:
		return v
	}
}

// computeChangedFields returns a description of what changed
func computeChangedFields(desired map[string]interface{}, actual map[string]interface{}) string {
	var changed []string

	for key, desiredValue := range desired {
		actualValue, exists := actual[key]
		if !exists {
			changed = append(changed, fmt.Sprintf("%s added", key))
		} else if !reflect.DeepEqual(normalizeValue(desiredValue), normalizeValue(actualValue)) {
			changed = append(changed, fmt.Sprintf("%s changed", key))
		}
	}

	if len(changed) == 0 {
		return "properties changed"
	}

	return fmt.Sprintf("%v", changed)
}
