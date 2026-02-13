package cli

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/graph"
	"context"
	"fmt"
)

func (c *CLI) Destroy(autoApprove bool) error {
	if err := c.stateManager.Lock(); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer c.stateManager.Unlock()

	currentState, err := c.stateManager.Load()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if len(currentState.Resources) == 0 {
		fmt.Println("No resources to destroy.")
		return nil
	}

	// Build a graph from state to get reverse dependency order
	var resources []*config.Resource
	for id, res := range currentState.Resources {
		resources = append(resources, &config.Resource{
			ID:         id,
			Type:       res.Type,
			Properties: res.Attributes,
		})
	}

	g := graph.NewGraph()
	if err := g.Build(resources); err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Use normal topological order for destroy (dependents first, then dependencies)
	// edges go dependent→dependency, so Kahn's gives dependents first
	order, err := g.TopologicalSort()
	if err != nil {
		return fmt.Errorf("failed to compute destroy order: %w", err)
	}

	// Display resources to be destroyed
	fmt.Println("\n=== Resources to Destroy ===")
	for _, id := range order {
		if res, ok := currentState.Resources[id]; ok {
			fmt.Printf("  - %s (%s)\n", id, res.Type)
		}
	}

	if !autoApprove {
		fmt.Print("\nDo you want to destroy all resources? (yes/no): ")
		var response string
		fmt.Scanln(&response)

		if response != "yes" {
			fmt.Println("Destroy cancelled.")
			return nil
		}
	}

	fmt.Println("\nDestroying resources...")

	ctx := context.Background()

	// Delete in reverse topological order (dependents first)
	var destroyErrors []string
	for _, id := range order {
		res, ok := currentState.Resources[id]
		if !ok {
			continue
		}

		prov, err := c.registry.Get(res.Type)
		if err != nil {
			destroyErrors = append(destroyErrors, fmt.Sprintf("%s: %v", id, err))
			continue
		}

		if err := prov.Delete(ctx, res.ID); err != nil {
			destroyErrors = append(destroyErrors, fmt.Sprintf("%s: %v", id, err))
			continue
		}

		fmt.Printf("Destroyed %s\n", id)
		delete(currentState.Resources, id)
	}

	// Always save state (even partial) so we don't lose track of remaining resources
	if err := c.stateManager.Save(currentState); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	if len(destroyErrors) > 0 {
		return fmt.Errorf("some resources failed to destroy: %v", destroyErrors)
	}

	fmt.Println("\nAll resources destroyed successfully!")

	return nil
}
