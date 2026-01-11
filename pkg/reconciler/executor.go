package reconciler

import (
	"GoIaC/pkg/provider"
	"GoIaC/pkg/state"
	"context"
	"fmt"
	"sync"
)

type ExecutionResult struct {
	Change   *Change
	NewState *state.ResourceState
	Err      error
}

// ExecuteChanges executes changes in parallel within each stage
func ExecuteChanges(
	ctx context.Context,
	changes []*Change,
	currentState *state.State,
	registry *provider.Registry,
) []ExecutionResult {

	results := make([]ExecutionResult, 0, len(changes))
	resultChan := make(chan ExecutionResult, len(changes))
	sem := make(chan struct{}, 4)

	var wg sync.WaitGroup

	for _, change := range changes {
		wg.Add(1)
		go func(change *Change) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			result := executeChange(ctx, change, currentState, registry)
			resultChan <- result
		}(change)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		results = append(results, result)
	}
	return results
}

// executeChange executes a single change
func executeChange(ctx context.Context, change *Change, currentState *state.State, registry *provider.Registry) ExecutionResult {
	switch change.Type {
	case ChangeTypeCreate:
		return executeCreate(ctx, change, currentState, registry)
	case ChangeTypeUpdate:
		return executeUpdate(ctx, change, currentState, registry)
	case ChangeTypeDelete:
		return executeDelete(ctx, change, registry)
	case ChangeTypeNoop:
		return ExecutionResult{Change: change, NewState: change.OldState}
	default:
		return ExecutionResult{Change: change, Err: fmt.Errorf("unknown change type: %s", change.Type)}
	}
}

func executeCreate(ctx context.Context, change *Change, currentState *state.State, registry *provider.Registry) ExecutionResult {
	resolved := InterpolateReferences(change.Resource, currentState)

	prov, err := registry.Get(change.Resource.Type)
	if err != nil {
		return ExecutionResult{
			Change: change,
			Err:    fmt.Errorf("provider not found: %w", err),
		}
	}

	newState, err := prov.Create(ctx, resolved)
	if err != nil {
		return ExecutionResult{
			Change: change,
			Err:    fmt.Errorf("create failed: %w", err),
		}
	}

	return ExecutionResult{
		Change:   change,
		NewState: newState,
	}
}

func executeUpdate(ctx context.Context, change *Change, currentState *state.State, registry *provider.Registry) ExecutionResult {
	resolved := InterpolateReferences(change.Resource, currentState)

	prov, err := registry.Get(change.Resource.Type)
	if err != nil {
		return ExecutionResult{
			Change: change,
			Err:    fmt.Errorf("provider not found: %w", err),
		}
	}

	newState, err := prov.Update(ctx, resolved, change.OldState.ID)
	if err != nil {
		return ExecutionResult{
			Change: change,
			Err:    fmt.Errorf("update failed: %w", err),
		}
	}

	return ExecutionResult{
		Change:   change,
		NewState: newState,
	}
}

func executeDelete(ctx context.Context, change *Change, registry *provider.Registry) ExecutionResult {
	prov, err := registry.Get(change.OldState.Type)
	if err != nil {
		return ExecutionResult{
			Change: change,
			Err:    fmt.Errorf("provider not found: %w", err),
		}
	}

	err = prov.Delete(ctx, change.OldState.ID)
	if err != nil {
		return ExecutionResult{
			Change: change,
			Err:    fmt.Errorf("deletion failed: %w", err),
		}
	}

	return ExecutionResult{
		Change:   change,
		NewState: nil, // Resource deleted
	}
}
