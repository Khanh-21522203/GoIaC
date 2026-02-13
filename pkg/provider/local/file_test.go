package local

import (
	"GoIaC/pkg/config"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileProvider_Create(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	provider := NewFileProvider()
	resource := &config.Resource{
		ID:   "test-file",
		Type: "local_file",
		Properties: map[string]interface{}{
			"path":    filePath,
			"content": "hello world",
		},
	}

	ctx := context.Background()
	state, err := provider.Create(ctx, resource)
	require.NoError(t, err)

	assert.Equal(t, filePath, state.ID)
	assert.Equal(t, "local_file", state.Type)
	assert.Equal(t, filePath, state.Attributes["path"])
	assert.Equal(t, "hello world", state.Attributes["content"])

	// Verify file was actually created
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestFileProvider_Create_MissingPath(t *testing.T) {
	provider := NewFileProvider()
	resource := &config.Resource{
		ID:   "test-file",
		Type: "local_file",
		Properties: map[string]interface{}{
			"content": "hello",
		},
	}

	ctx := context.Background()
	_, err := provider.Create(ctx, resource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path property required")
}

func TestFileProvider_Create_MissingContent(t *testing.T) {
	provider := NewFileProvider()
	resource := &config.Resource{
		ID:   "test-file",
		Type: "local_file",
		Properties: map[string]interface{}{
			"path": "/tmp/test.txt",
		},
	}

	ctx := context.Background()
	_, err := provider.Create(ctx, resource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "content property required")
}

func TestFileProvider_Read(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("existing content"), 0644))

	provider := NewFileProvider()
	ctx := context.Background()

	state, err := provider.Read(ctx, filePath)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, filePath, state.ID)
	assert.Equal(t, "existing content", state.Attributes["content"])
}

func TestFileProvider_Read_NotFound(t *testing.T) {
	provider := NewFileProvider()
	ctx := context.Background()

	state, err := provider.Read(ctx, "/nonexistent/path/file.txt")
	assert.NoError(t, err)
	assert.Nil(t, state, "Read of non-existent file should return (nil, nil)")
}

func TestFileProvider_Update(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("old content"), 0644))

	provider := NewFileProvider()
	resource := &config.Resource{
		ID:   "test-file",
		Type: "local_file",
		Properties: map[string]interface{}{
			"path":    filePath,
			"content": "new content",
		},
	}

	ctx := context.Background()
	state, err := provider.Update(ctx, resource, filePath)
	require.NoError(t, err)

	assert.Equal(t, "new content", state.Attributes["content"])

	// Verify file was actually updated
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(data))
}

func TestFileProvider_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("to delete"), 0644))

	provider := NewFileProvider()
	ctx := context.Background()

	err := provider.Delete(ctx, filePath)
	require.NoError(t, err)

	// Verify file was deleted
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}

func TestFileProvider_Delete_Idempotent(t *testing.T) {
	provider := NewFileProvider()
	ctx := context.Background()

	// Deleting a non-existent file should not error (idempotent)
	err := provider.Delete(ctx, "/nonexistent/path/file.txt")
	assert.NoError(t, err)
}

func TestFileProvider_CreateReadUpdateDelete(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "lifecycle.txt")
	provider := NewFileProvider()
	ctx := context.Background()

	// Create
	resource := &config.Resource{
		ID:   "lifecycle",
		Type: "local_file",
		Properties: map[string]interface{}{
			"path":    filePath,
			"content": "v1",
		},
	}
	createState, err := provider.Create(ctx, resource)
	require.NoError(t, err)
	assert.Equal(t, "v1", createState.Attributes["content"])

	// Read
	readState, err := provider.Read(ctx, filePath)
	require.NoError(t, err)
	assert.Equal(t, "v1", readState.Attributes["content"])

	// Update
	resource.Properties["content"] = "v2"
	updateState, err := provider.Update(ctx, resource, filePath)
	require.NoError(t, err)
	assert.Equal(t, "v2", updateState.Attributes["content"])

	// Delete
	err = provider.Delete(ctx, filePath)
	require.NoError(t, err)

	// Read after delete
	readState, err = provider.Read(ctx, filePath)
	assert.NoError(t, err)
	assert.Nil(t, readState)
}
