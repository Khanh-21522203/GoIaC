package cli

import "fmt"

func (c *CLI) StateShow(resourceID string) error {
	// Load state
	currentState, err := c.stateManager.Load()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if len(currentState.Resources) == 0 {
		fmt.Println("No resources in state.")
		return nil
	}

	if resourceID != "" {
		// Show specific resource
		res, ok := currentState.Resources[resourceID]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceID)
		}

		fmt.Printf("\nResource: %s\n", resourceID)
		fmt.Printf("Type: %s\n", res.Type)
		fmt.Printf("ID: %s\n", res.ID)
		fmt.Println("Attributes:")
		for k, v := range res.Attributes {
			fmt.Printf("  %s: %v\n", k, v)
		}
	} else {
		// Show all resources
		fmt.Println("\n=== Current State ===")
		for id, res := range currentState.Resources {
			fmt.Printf("%s (%s)\n", id, res.Type)
			fmt.Printf("  ID: %s\n", res.ID)
			for k, v := range res.Attributes {
				fmt.Printf("  %s: %v\n", k, v)
			}
			fmt.Println()
		}
	}

	return nil
}
