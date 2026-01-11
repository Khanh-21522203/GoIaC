package local

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/state"
	"context"
	"fmt"
	"os"
)

type FileProvider struct{}

func NewFileProvider() *FileProvider {
	return &FileProvider{}
}

func (p *FileProvider) Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error) {
	path, ok := desired.Properties["path"].(string)
	if !ok {
		return nil, fmt.Errorf("path property required")
	}

	content, ok := desired.Properties["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content property required")
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return &state.ResourceState{
		ID:   path,
		Type: desired.Type,
		Attributes: map[string]interface{}{
			"path":    path,
			"content": content,
			"size":    info.Size(),
			"mode":    info.Mode().String(),
		},
	}, nil
}

func (p *FileProvider) Read(ctx context.Context, resourceID string) (*state.ResourceState, error) {
	info, err := os.Stat(resourceID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	content, err := os.ReadFile(resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return &state.ResourceState{
		ID:   resourceID,
		Type: "local_file",
		Attributes: map[string]interface{}{
			"path":    resourceID,
			"content": string(content),
			"size":    info.Size(),
			"mode":    info.Mode().String(),
		},
	}, nil
}

func (p *FileProvider) Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error) {
	content, ok := desired.Properties["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content property required")
	}

	if err := os.WriteFile(resourceID, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	return p.Read(ctx, resourceID)
}

func (p *FileProvider) Delete(ctx context.Context, resourceID string) error {
	if err := os.Remove(resourceID); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
