package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(path string) ([]*Resource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := p.validate(config.Resources); err != nil {
		return nil, err
	}

	return config.Resources, nil
}

func (p *Parser) validate(resources []*Resource) error {
	if len(resources) == 0 {
		return fmt.Errorf("no resources defined in configuration")
	}

	seen := make(map[string]bool)
	for _, resource := range resources {
		if resource.ID == "" {
			return fmt.Errorf("resource missing ID")
		}
		if resource.Type == "" {
			return fmt.Errorf("resource %s missing type", resource.ID)
		}
		if seen[resource.ID] {
			return fmt.Errorf("duplicate resource %s", resource.ID)
		}
		seen[resource.ID] = true
	}

	return nil
}

// ParseString parses YAML from a string (useful for testing)
func (p *Parser) ParseString(yamlStr string) ([]*Resource, error) {
	var config Config
	if err := yaml.Unmarshal([]byte(yamlStr), &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := p.validate(config.Resources); err != nil {
		return nil, err
	}

	return config.Resources, nil
}
