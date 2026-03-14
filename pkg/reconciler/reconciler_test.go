package reconciler

import (
	"GoIaC/pkg/config"
	"GoIaC/pkg/provider"
	"GoIaC/pkg/state"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider is a configurable test double for provider.Provider.
type mockProvider struct {
	createFn func(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)
	deleteFn func(ctx context.Context, resourceID string) error
}

func (m *mockProvider) Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error) {
	if m.createFn != nil {
		return m.createFn(ctx, desired)
	}
	return &state.ResourceState{
		ID:         desired.ID,
		Type:       desired.Type,
		Attributes: desired.Properties,
	}, nil
}

func (m *mockProvider) Read(ctx context.Context, resourceID string) (*state.ResourceState, error) {
	return nil, nil
}

func (m *mockProvider) Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error) {
	return &state.ResourceState{
		ID:         resourceID,
		Type:       desired.Type,
		Attributes: desired.Properties,
	}, nil
}

func (m *mockProvider) Delete(ctx context.Context, resourceID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, resourceID)
	}
	return nil
}

func newTestReconciler(t *testing.T, prov provider.Provider) (*Reconciler, *state.Manager) {
	t.Helper()
	mgr := state.NewManagerWithDir(t.TempDir())
	reg := provider.NewRegistry()
	reg.Register("local_file", prov)
	return NewReconciler(mgr, reg), mgr
}

func TestReconciler_PlanValidatesResources(t *testing.T) {
	rec, _ := newTestReconciler(t, &mockProvider{})
	desired := []*config.Resource{
		{ID: "f", Type: "unknown_type", Properties: map[string]interface{}{}},
	}
	_, err := rec.Plan(desired)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown resource type")
}

func TestReconciler_PlanDetectsCycle(t *testing.T) {
	rec, _ := newTestReconciler(t, &mockProvider{})
	// local_file doesn't normally have cross-references; inject fake ones via
	// a pre-built desired list whose properties contain cyclic ${...} refs.
	desired := []*config.Resource{
		{
			ID:   "a",
			Type: "local_file",
			Properties: map[string]interface{}{
				"path":    "a.txt",
				"content": "${local_file.b.path}",
			},
		},
		{
			ID:   "b",
			Type: "local_file",
			Properties: map[string]interface{}{
				"path":    "b.txt",
				"content": "${local_file.a.path}",
			},
		},
	}
	_, err := rec.Plan(desired)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestReconciler_ApplyCreatesResource(t *testing.T) {
	var created string
	prov := &mockProvider{
		createFn: func(_ context.Context, desired *config.Resource) (*state.ResourceState, error) {
			created = desired.ID
			return &state.ResourceState{
				ID:         desired.ID,
				Type:       desired.Type,
				Attributes: desired.Properties,
			}, nil
		},
	}
	rec, mgr := newTestReconciler(t, prov)

	desired := []*config.Resource{
		{ID: "f", Type: "local_file", Properties: map[string]interface{}{"path": "a.txt", "content": "hello"}},
	}
	err := rec.Apply(context.Background(), desired)
	require.NoError(t, err)
	assert.Equal(t, "f", created)

	// Verify state was persisted
	s, err := mgr.Load()
	require.NoError(t, err)
	assert.Contains(t, s.Resources, "f")
}

func TestReconciler_ApplyDeletesResource(t *testing.T) {
	var deleted string
	prov := &mockProvider{
		deleteFn: func(_ context.Context, resourceID string) error {
			deleted = resourceID
			return nil
		},
	}
	rec, mgr := newTestReconciler(t, prov)

	// Seed state with an existing resource
	s := state.NewState()
	s.Resources["old"] = &state.ResourceState{ID: "old-id", Type: "local_file", Attributes: map[string]interface{}{"path": "old.txt", "content": "x"}}
	require.NoError(t, mgr.Save(s))

	// Apply with empty desired — should delete "old"
	err := rec.Apply(context.Background(), []*config.Resource{})
	require.NoError(t, err)
	assert.Equal(t, "old-id", deleted)

	// Verify removed from state
	loaded, err := mgr.Load()
	require.NoError(t, err)
	assert.NotContains(t, loaded.Resources, "old")
}

func TestReconciler_ApplyPartialStateOnFailure(t *testing.T) {
	callCount := 0
	prov := &mockProvider{
		createFn: func(_ context.Context, desired *config.Resource) (*state.ResourceState, error) {
			callCount++
			if callCount == 2 {
				return nil, fmt.Errorf("provider error on second resource")
			}
			return &state.ResourceState{
				ID:         desired.ID,
				Type:       desired.Type,
				Attributes: desired.Properties,
			}, nil
		},
	}
	rec, mgr := newTestReconciler(t, prov)

	desired := []*config.Resource{
		{ID: "first", Type: "local_file", Properties: map[string]interface{}{"path": "a.txt", "content": "a"}},
		{ID: "second", Type: "local_file", Properties: map[string]interface{}{"path": "b.txt", "content": "b"}},
	}
	err := rec.Apply(context.Background(), desired)
	assert.Error(t, err)

	// Partial state: exactly one resource (the first to be applied) should be saved.
	// Topological order for independent resources is non-deterministic, so we check
	// that exactly one of the two is in state and the other is not.
	loaded, err := mgr.Load()
	require.NoError(t, err)
	assert.Len(t, loaded.Resources, 1, "exactly one resource should be saved in partial state")
	firstInState := loaded.Resources["first"] != nil
	secondInState := loaded.Resources["second"] != nil
	assert.True(t, firstInState != secondInState, "exactly one of first/second should be in state")
}
