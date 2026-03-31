package component

import (
	"errors"
	"testing"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewComponentBuilder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		componentName string
		conditionType ConditionType
		wantErr       bool
		expectedErr   string
	}{
		{
			name:          "Valid builder",
			componentName: "test-component",
			conditionType: "TestReady",
			wantErr:       false,
		},
		{
			name:          "Empty component name",
			componentName: "",
			conditionType: "TestReady",
			wantErr:       true,
			expectedErr:   "component name cannot be empty",
		},
		{
			name:          "Empty condition type",
			componentName: "test-component",
			conditionType: "",
			wantErr:       true,
			expectedErr:   "condition type cannot be empty",
		},
		{
			name:          "Missing component name",
			componentName: "skip",
			conditionType: "TestReady",
			wantErr:       true,
			expectedErr:   "component name must be supplied",
		},
		{
			name:          "Missing condition type",
			componentName: "test-component",
			conditionType: "skip",
			wantErr:       true,
			expectedErr:   "condition type must be supplied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := NewComponentBuilder()
			if tt.componentName != "skip" {
				b.WithName(tt.componentName)
			}
			if tt.conditionType != "skip" {
				b.WithConditionType(tt.conditionType)
			}

			comp, err := b.Build()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, comp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, comp)
				assert.Equal(t, tt.componentName, comp.name)
				assert.Equal(t, tt.conditionType, comp.conditionType)
				assert.Equal(t, false, comp.suspended)
			}
		})
	}
}

func TestBuilder_WithResource(t *testing.T) {
	t.Parallel()

	res1 := &MockResource{}
	res1.On("Identity").Return("v1/Pod/res1")
	res2 := &MockResource{}
	res2.On("Identity").Return("v1/Service/res2")
	res3 := &MockResource{}
	res3.On("Identity").Return("v1/ConfigMap/res3")

	b := NewComponentBuilder()
	b.WithName("test").WithConditionType("Ready")

	b.WithResource(res1, ResourceOptions{Delete: false, ReadOnly: false}) // create
	b.WithResource(res2, ResourceOptions{Delete: false, ReadOnly: true})  // read-only
	b.WithResource(res3, ResourceOptions{Delete: true, ReadOnly: false})  // delete

	comp, err := b.Build()
	require.NoError(t, err)

	assert.Len(t, comp.reconcileResources, 2)
	assert.Equal(t, res1, comp.reconcileResources[0].Resource)
	assert.False(t, comp.reconcileResources[0].Options.ReadOnly)
	assert.Equal(t, res2, comp.reconcileResources[1].Resource)
	assert.True(t, comp.reconcileResources[1].Options.ReadOnly)

	assert.Len(t, comp.deleteResources, 1)
	assert.Equal(t, res3, comp.deleteResources[0])

	assert.Len(t, comp.resourceLookup, 3)
}

func TestBuilder_DuplicateResource(t *testing.T) {
	t.Parallel()

	res1 := &MockResource{}
	res1.On("Identity").Return("v1/Pod/res1")

	b := NewComponentBuilder()
	b.WithName("test").WithConditionType("Ready")

	b.WithResource(res1, ResourceOptions{})
	b.WithResource(res1, ResourceOptions{})

	comp, err := b.Build()
	require.Error(t, err)
	assert.Nil(t, comp)
	assert.Contains(t, err.Error(), "duplicate resource \"v1/Pod/res1\" in component \"test\"")
}

func TestBuilder_WithGracePeriod(t *testing.T) {
	t.Parallel()

	t.Run("Valid grace period", func(t *testing.T) {
		t.Parallel()
		b := NewComponentBuilder().WithName("test").WithConditionType("Ready")

		b.WithGracePeriod(5 * time.Minute)
		comp, err := b.Build()
		require.NoError(t, err)
		assert.Equal(t, 5*time.Minute, comp.gracePeriod)
	})

	t.Run("Negative grace period", func(t *testing.T) {
		t.Parallel()
		b := NewComponentBuilder().WithName("test").WithConditionType("Ready")

		b.WithGracePeriod(-1 * time.Second)
		comp, err := b.Build()
		require.Error(t, err)
		assert.Nil(t, comp)
		assert.Contains(t, err.Error(), "grace period must be positive")
	})
}

func TestBuilder_WithFeatureGate(t *testing.T) {
	t.Parallel()

	t.Run("Valid gate", func(t *testing.T) {
		t.Parallel()
		gate := feature.NewVersionGate("1.0.0", nil)
		b := NewComponentBuilder().WithName("test").WithConditionType("Ready")

		b.WithFeatureGate(gate)
		comp, err := b.Build()
		require.NoError(t, err)
		assert.NotNil(t, comp.featureGate)
	})

	t.Run("Nil gate is no-op", func(t *testing.T) {
		t.Parallel()
		b := NewComponentBuilder().WithName("test").WithConditionType("Ready")

		b.WithFeatureGate(nil)
		comp, err := b.Build()
		require.NoError(t, err)
		assert.Nil(t, comp.featureGate)
	})

	t.Run("Calling twice records error", func(t *testing.T) {
		t.Parallel()
		gate := feature.NewVersionGate("1.0.0", nil)
		b := NewComponentBuilder().WithName("test").WithConditionType("Ready")

		b.WithFeatureGate(gate).WithFeatureGate(gate)
		_, err := b.Build()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "feature gate already set")
	})
}

func TestBuilder_WithPrerequisite(t *testing.T) {
	t.Parallel()

	t.Run("Valid prerequisite", func(t *testing.T) {
		t.Parallel()
		prereq := &testPrerequisite{}
		b := NewComponentBuilder().WithName("test").WithConditionType("Ready")

		b.WithPrerequisite(prereq)
		comp, err := b.Build()
		require.NoError(t, err)
		assert.Len(t, comp.prerequisites, 1)
	})

	t.Run("Multiple prerequisites", func(t *testing.T) {
		t.Parallel()
		b := NewComponentBuilder().WithName("test").WithConditionType("Ready")

		b.WithPrerequisite(&testPrerequisite{}).WithPrerequisite(&testPrerequisite{})
		comp, err := b.Build()
		require.NoError(t, err)
		assert.Len(t, comp.prerequisites, 2)
	})

	t.Run("Nil prerequisite is no-op", func(t *testing.T) {
		t.Parallel()
		b := NewComponentBuilder().WithName("test").WithConditionType("Ready")

		b.WithPrerequisite(nil)
		comp, err := b.Build()
		require.NoError(t, err)
		assert.Empty(t, comp.prerequisites)
	})
}

type testPrerequisite struct{}

func (t *testPrerequisite) Check(_ ReconcileContext) (PrerequisiteResult, error) {
	return PrerequisiteResult{Status: PrerequisiteStatusMet}, nil
}

func TestBuilder_BuildErrorAggregation(t *testing.T) {
	t.Parallel()

	res1 := &MockResource{}
	res1.On("Identity").Return("v1/Pod/res1")

	b := NewComponentBuilder()
	b.WithName("test").WithConditionType("Ready")

	b.WithResource(res1, ResourceOptions{})
	b.WithResource(res1, ResourceOptions{}) // Duplicate error
	b.WithGracePeriod(-1 * time.Second)     // Grace period error

	comp, err := b.Build()
	require.Error(t, err)
	assert.Nil(t, comp)

	// Check if both errors are present
	var joinedErr interface{ Unwrap() []error }
	if errors.As(err, &joinedErr) {
		errs := joinedErr.Unwrap()
		assert.Len(t, errs, 2)
	} else {
		// Fallback for older Go or different error joining
		assert.Contains(t, err.Error(), "duplicate resource")
		assert.Contains(t, err.Error(), "grace period must be positive")
	}
}
