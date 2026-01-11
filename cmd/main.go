package main

import (
	"GoIaC/pkg/cli"
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		runInteractiveMode()
		return
	}

	cliInstance, err := cli.NewCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cmdErr := executeCommand(cliInstance, os.Args[1:])

	if cmdErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", cmdErr)
		os.Exit(1)
	}
}

func runInteractiveMode() {
	printUsage()
	fmt.Println()
	fmt.Println("Interactive mode - Type a command or 'exit' to quit")

	cliInstance, err := cli.NewCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing CLI: %v\n", err)
		return
	}

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("\ngoiac> ")

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		args := strings.Fields(input)
		if err := executeCommand(cliInstance, args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
	}
}

func executeCommand(cliInstance *cli.CLI, args []string) error {
	if len(args) == 0 {
		return nil
	}

	command := args[0]

	switch command {
	case "init":
		return cliInstance.Init()
	case "plan":
		configPath := "main.yaml"
		if len(args) > 1 {
			configPath = args[1]
		}
		return cliInstance.Plan(configPath)
	case "apply":
		configPath := "main.yaml"
		if len(args) > 1 {
			configPath = args[1]
		}
		return cliInstance.Apply(configPath)
	case "destroy":
		return cliInstance.Destroy()
	case "state":
		if len(args) > 1 && args[1] == "show" {
			resourceID := ""
			if len(args) > 2 {
				resourceID = args[2]
			}
			return cliInstance.StateShow(resourceID)
		} else {
			fmt.Println("Usage: goiac state show [resource-id]")
			return nil
		}
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		return nil
	}
}

func printUsage() {
	fmt.Println("GoIaC - A Minimal Infrastructure-as-Code Engine")
	fmt.Println()
	fmt.Println("Usage: goiac [command] [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init              Initialize new GoIaC project")
	fmt.Println("  plan [config]     Generate execution plan (default: main.yaml)")
	fmt.Println("  apply [config]    Apply changes to infrastructure")
	fmt.Println("  destroy           Delete all managed resources")
	fmt.Println("  state show [id]   Display current state")
	fmt.Println("  help              Show this help message")
}
