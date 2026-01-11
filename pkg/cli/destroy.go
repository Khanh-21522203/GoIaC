package cli

import (
	"context"
	"fmt"
)

func (c *CLI) Destroy() error {
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

	// Display resources to be destroyed
	fmt.Println("\n=== Resources to Destroy ===\n")
	for id, res := range currentState.Resources {
		fmt.Printf("  - %s (%s)\n", id, res.Type)
	}

	fmt.Print("\nDo you want to destroy all resources? (yes/no): ")
	var response string
	fmt.Scanln(&response)

	if response != "yes" {
		fmt.Println("Destroy cancelled.")
		return nil
	}

	fmt.Println("\nDestroying resources...")

	ctx := context.Background()

	// Delete in reverse order (no dependencies)
	for id, res := range currentState.Resources {
		prov, err := c.registry.Get(res.Type)
		if err != nil {
			fmt.Printf("Failed to get provider for %s: %v\n", id, err)
			continue
		}

		if err := prov.Delete(ctx, res.ID); err != nil {
			fmt.Printf("Failed to delete %s: %v\n", id, err)
			continue
		}

		fmt.Printf("Destroyed %s\n", id)
		delete(currentState.Resources, id)
	}

	if err := c.stateManager.Save(currentState); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	fmt.Println("\nAll resources destroyed successfully!")

	return nil
}
