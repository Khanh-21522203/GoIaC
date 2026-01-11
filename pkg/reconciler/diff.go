package reconciler

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/state"
	"fmt"
)

type ChangeType int

const (
	ChangeTypeCreate ChangeType = iota
	ChangeTypeUpdate
	ChangeTypeDelete
	ChangeTypeNoop
)

type Change struct {
	Type     ChangeType
	Resource *config.Resource
	OldState *state.ResourceState
	Reason   string
}

func ComputeDiff(desired []*config.Resource, acutal *state.State) []*Change {
	var changes []*Change
	processed := make(map[string]bool)

	for _, resource := range desired {
		processed[resource.ID] = true

		oldState, exists := acutal.Resources[resource.ID]
		if !exists {
			changes = append(changes, &Change{
				Type:     ChangeTypeCreate,
				Resource: resource,
				Reason:   "resource does not exist",
			})
		} else if propertiesDiffer(resource.Properties, oldState.Attributes) {
			changes = append(changes, &Change{
				Type:     ChangeTypeUpdate,
				Resource: resource,
				OldState: oldState,
				Reason:   computeChangedFields(resource.Properties, oldState.Attributes),
			})
		} else {
			changes = append(changes, &Change{
				Type:     ChangeTypeNoop,
				Resource: resource,
				OldState: oldState,
			})
		}
	}

	// Check for resources to delete (in actual but not desired)
	for resourceID, oldState := range acutal.Resources {
		if !processed[resourceID] {
			changes = append(changes, &Change{
				Type:     ChangeTypeDelete,
				OldState: oldState,
				Reason:   "resource no longer in configuration",
			})
		}
	}

	return changes
}

// propertiesDiffer checks if properties differ from attributes
func propertiesDiffer(desired map[string]interface{}, actual map[string]interface{}) bool {
	for key, desiredValue := range desired {
		actualValue, exists := actual[key]
		if !exists || fmt.Sprint(desiredValue) != fmt.Sprint(actualValue) {
			return true
		}
	}
	return false
}

// computeChangedFields returns a description of what changed
func computeChangedFields(desired map[string]interface{}, actual map[string]interface{}) string {
	var changed []string

	for key, desiredValue := range desired {
		actualValue, exists := actual[key]
		if !exists {
			changed = append(changed, fmt.Sprintf("%s added", key))
		} else if fmt.Sprint(desiredValue) != fmt.Sprint(actualValue) {
			changed = append(changed, fmt.Sprintf("%s changed", key))
		}
	}

	if len(changed) == 0 {
		return "properties changed"
	}

	return fmt.Sprintf("%v", changed)
}
