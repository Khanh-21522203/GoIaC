package cli

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/provider"
	"GoIaC/pkg/provider/docker"
	"GoIaC/pkg/provider/local"
	"GoIaC/pkg/reconciler"
	"GoIaC/pkg/state"
	"fmt"
)

type CLI struct {
	stateManager *state.Manager
	parser       *config.Parser
	registry     *provider.Registry
	reconciler   *reconciler.Reconciler
}

func NewCLI() (*CLI, error) {
	stateManager := state.NewManager()
	parser := config.NewParser()
	registry := provider.NewRegistry()

	if err := registerProviders(registry); err != nil {
		return nil, fmt.Errorf("failed to register providers: %w", err)
	}

	reconcilerInstance := reconciler.NewReconciler(stateManager, registry)

	return &CLI{
		stateManager: stateManager,
		parser:       parser,
		registry:     registry,
		reconciler:   reconcilerInstance,
	}, nil
}

// registerProviders registers all built-in providers
func registerProviders(registry *provider.Registry) error {
	// Local file provider
	registry.Register("local_file", local.NewFileProvider())

	// Docker container provider
	containerProvider, err := docker.NewContainerProvider()
	if err != nil {
		return fmt.Errorf("failed to create container provider: %w", err)
	}
	registry.Register("docker_container", containerProvider)

	// Docker network provider
	networkProvider, err := docker.NewNetworkProvider()
	if err != nil {
		return fmt.Errorf("failed to create network provider: %w", err)
	}
	registry.Register("docker_network", networkProvider)

	return nil
}
