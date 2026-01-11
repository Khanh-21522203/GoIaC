package provider

import (
	"fmt"
	"sync"
)

type Registry struct {
	providers map[string]Provider
	mu        sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

func (r *Registry) Register(resourceType string, provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[resourceType] = provider
}

func (r *Registry) Get(resourceType string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[resourceType]
	if !ok {
		return nil, fmt.Errorf("provider not found for resource type: %s", resourceType)
	}

	return provider, nil
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.providers))
	for resourceType := range r.providers {
		types = append(types, resourceType)
	}

	return types
}
