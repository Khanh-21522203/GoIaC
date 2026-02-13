package reconciler

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/graph"
	"GoIaC/pkg/logger"
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
	log := logger.Get()

	// Validate resource properties against known schemas
	if err := provider.ValidateResources(desired); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	log.Info("loading current state")

	currentState, err := r.stateManager.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	log.Info("computing diff", "desired_count", len(desired), "current_count", len(currentState.Resources))
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

	log.Info("plan complete", "changes", len(changes))
	return changes, nil
}

func (r *Reconciler) Apply(ctx context.Context, desired []*config.Resource) error {
	// Validate resource properties against known schemas
	if err := provider.ValidateResources(desired); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

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

	// Use reverse topological order so dependencies are created before dependents
	// (edges go dependent→dependency, so reverse gives dependency-first order)
	order, err := g.TopologicalSortReverse()
	if err != nil {
		return fmt.Errorf("failed to build topological order: %w", err)
	}

	log := logger.Get()

	// Execute changes in dependency order
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

		log.Info("executing changes", "resource", resourceID, "count", len(resourceChanges))
		results := ExecuteChanges(ctx, resourceChanges, currentState, r.registry)

		for _, result := range results {
			if result.Err != nil {
				log.Error("change failed", "resource", result.Change.Resource.ID, "error", result.Err)
				// Save partial state before returning error
				if saveErr := r.stateManager.Save(currentState); saveErr != nil {
					log.Error("failed to save partial state", "error", saveErr)
				}
				return fmt.Errorf("failed to execute change for %s: %w",
					result.Change.Resource.ID, result.Err)
			}

			if result.NewState != nil {
				currentState.Resources[result.Change.Resource.ID] = result.NewState
			}
		}
	}

	// Handle deletions (resources in state but not in desired config)
	for _, change := range changes {
		if change.Type == ChangeTypeDelete {
			results := ExecuteChanges(ctx, []*Change{change}, currentState, r.registry)
			for _, result := range results {
				if result.Err != nil {
					return fmt.Errorf("failed to delete %s: %w",
						result.Change.OldState.ID, result.Err)
				}
				// Use OldState.ID to find the key — Resource is nil for deletes
				for key, res := range currentState.Resources {
					if res.ID == result.Change.OldState.ID {
						delete(currentState.Resources, key)
						break
					}
				}
			}
		}
	}

	return r.stateManager.Save(currentState)
}
