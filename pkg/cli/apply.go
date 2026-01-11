package cli

import (
	"context"
	"fmt"
)

func (c *CLI) Apply(configPath string) error {
	if err := c.stateManager.Lock(); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer c.stateManager.Unlock()

	desired, err := c.parser.Parse(configPath)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	changes, err := c.reconciler.Plan(desired)
	if err != nil {
		return fmt.Errorf("failed to apply changes: %w", err)
	}

	c.printPlan(changes)

	fmt.Print("\nDo you want to apply these changes? (yes/no): ")
	var response string
	fmt.Scanln(&response)

	if response != "yes" {
		fmt.Println("Apply cancelled.")
		return nil
	}

	fmt.Println("\nApplying changes...")

	ctx := context.Background()
	if err := c.reconciler.Apply(ctx, desired); err != nil {
		return fmt.Errorf("failed to apply changes: %w", err)
	}

	fmt.Println("\nInfrastructure updated successfully!")

	return nil
}
