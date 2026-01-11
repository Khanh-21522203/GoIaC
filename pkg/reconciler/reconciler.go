package reconciler

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/graph"
	"GoIaC/pkg/provider"
	"GoIaC/pkg/state"
	"context"
	"fmt"
)

type Reconciler struct {
	stateManager *state.Manager
	registry     *provider.Registry
}

func NewReconciler(stateManager *state.Manager, registry *provider.Registry) *Reconciler {
	return &Reconciler{
		stateManager: stateManager,
		registry:     registry,
	}
}

func (r *Reconciler) Plan(desired []*config.Resource) ([]*Change, error) {
	currentState, err := r.stateManager.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	changes := ComputeDiff(desired, currentState)

	g := graph.NewGraph()
	if err := g.Build(desired); err != nil {
		return nil, fmt.Errorf("failed to build graph: %w", err)
	}

	if err := g.ValidateDAG(); err != nil {
		return nil, fmt.Errorf("failed to validate dag: %w", err)
	}

	if err := g.ValidateReferences(); err != nil {
		return nil, fmt.Errorf("failed to validate references: %w", err)
	}

	return changes, nil
}

func (r *Reconciler) Apply(ctx context.Context, desired []*config.Resource) error {
	currentState, err := r.stateManager.Load()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	changes := ComputeDiff(desired, currentState)

	g := graph.NewGraph()
	if err := g.Build(desired); err != nil {
		return fmt.Errorf("failed to build graph: %w", err)
	}
	if err := g.ValidateDAG(); err != nil {
		return fmt.Errorf("failed to validate dag: %w", err)
	}
	if err := g.ValidateReferences(); err != nil {
		return fmt.Errorf("failed to validate references: %w", err)
	}

	order, err := g.TopologicalSort()
	if err != nil {
		return fmt.Errorf("failed to build topological order: %w", err)
	}

	// Execute changes in order
	for _, resourceID := range order {
		var resourceChanges []*Change
		for _, change := range changes {
			if change.Type == ChangeTypeNoop {
				continue
			}
			if change.Resource != nil && change.Resource.ID == resourceID {
				resourceChanges = append(resourceChanges, change)
			}
		}

		if len(resourceChanges) == 0 {
			continue
		}

		results := ExecuteChanges(ctx, resourceChanges, currentState, r.registry)

		for _, result := range results {
			if result.Err != nil {
				return fmt.Errorf("failed to execute change for %s: %w",
					result.Change.Resource.ID, result.Err)
			}

			if result.NewState != nil {
				currentState.Resources[result.Change.Resource.ID] = result.NewState
			}
		}
	}

	// Handle deletions (in reverse order)
	for _, change := range changes {
		if change.Type == ChangeTypeDelete {
			results := ExecuteChanges(ctx, []*Change{change}, currentState, r.registry)
			for _, result := range results {
				if result.Err != nil {
					return fmt.Errorf("failed to delete %s: %w",
						result.Change.OldState.ID, result.Err)
				}
				delete(currentState.Resources, result.Change.Resource.ID)
			}
		}
	}

	return r.stateManager.Save(currentState)
}
