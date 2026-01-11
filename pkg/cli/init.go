package cli

import (
	"fmt"
	"os"
)

func (c *CLI) Init() error {
	if err := os.MkdirAll(".goiac", 0755); err != nil {
		return fmt.Errorf("failed to create .goiac directory: %w", err)
	}

	fmt.Println("Created .goiac directory")

	// Create example config if main.yaml doesn't exist
	if _, err := os.Stat("main.yaml"); os.IsNotExist(err) {
		exampleConfig := `resources:
  - id: example
    type: local_file
    properties:
      path: ./example.txt
      content: "Hello from MyIaC!"
`
		if err := os.WriteFile("main.yaml", []byte(exampleConfig), 0644); err != nil {
			return fmt.Errorf("failed to create example config: %w", err)
		}
		fmt.Println("✓ Created example main.yaml")
	}

	fmt.Println("\nProject initialized successfully!")
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit main.yaml to define your infrastructure")
	fmt.Println("  2. Run 'myiac plan' to preview changes")
	fmt.Println("  3. Run 'myiac apply' to create resources")

	return nil
}
