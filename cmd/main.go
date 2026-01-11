package main

import (
	"GoIaC/pkg/cli"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	cliInstance, err := cli.NewCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var cmdErr error

	switch command {
	case "init":
		cmdErr = cliInstance.Init()
	case "plan":
		configPath := "main.yaml"
		if len(os.Args) > 2 {
			configPath = os.Args[2]
		}
		cmdErr = cliInstance.Plan(configPath)
	case "apply":
		configPath := "main.yaml"
		if len(os.Args) > 2 {
			configPath = os.Args[2]
		}
		cmdErr = cliInstance.Apply(configPath)
	case "destroy":
		cmdErr = cliInstance.Destroy()
	case "state":
		if len(os.Args) > 2 && os.Args[2] == "show" {
			resourceID := ""
			if len(os.Args) > 3 {
				resourceID = os.Args[3]
			}
			cmdErr = cliInstance.StateShow(resourceID)
		} else {
			fmt.Println("Usage: myiac state show [resource-id]")
			os.Exit(1)
		}
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}

	if cmdErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", cmdErr)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("MyIaC - A Minimal Infrastructure-as-Code Engine")
	fmt.Println()
	fmt.Println("Usage: myiac [command] [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init              Initialize new MyIaC project")
	fmt.Println("  plan [config]     Generate execution plan (default: main.yaml)")
	fmt.Println("  apply [config]    Apply changes to infrastructure")
	fmt.Println("  destroy           Delete all managed resources")
	fmt.Println("  state show [id]   Display current state")
	fmt.Println("  help              Show this help message")
}
