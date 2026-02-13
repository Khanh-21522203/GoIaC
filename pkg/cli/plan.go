package cli

import (
	"GoIaC/pkg/reconciler"
	"fmt"
)

func (c *CLI) Plan(configPath string) error {
	desired, err := c.parser.Parse(configPath)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	changes, err := c.reconciler.Plan(desired)
	if err != nil {
		return fmt.Errorf("failed to generate plan: %w", err)
	}

	c.printPlan(changes)

	return nil
}

// printPlan displays the execution plan
func (c *CLI) printPlan(changes []*reconciler.Change) {
	fmt.Println("\n=== Execution Plan ===")

	createCount := 0
	updateCount := 0
	deleteCount := 0
	noopCount := 0

	for _, change := range changes {
		switch change.Type {
		case reconciler.ChangeTypeCreate:
			fmt.Printf("  + Create: %s (%s)\n", change.Resource.ID, change.Resource.Type)
			createCount++
		case reconciler.ChangeTypeUpdate:
			fmt.Printf("  ~ Update: %s\n", change.Resource.ID)
			fmt.Printf("      Reason: %s\n", change.Reason)
			updateCount++
		case reconciler.ChangeTypeDelete:
			fmt.Printf("  - Delete: %s\n", change.OldState.ID)
			deleteCount++
		case reconciler.ChangeTypeNoop:
			noopCount++
		}
	}

	fmt.Printf("\nSummary: %d to create, %d to update, %d to delete, %d unchanged\n",
		createCount, updateCount, deleteCount, noopCount)
}
