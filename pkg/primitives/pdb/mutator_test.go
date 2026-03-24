package pdb

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func newTestPDB() *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pdb",
			Namespace: "default",
		},
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", p.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditSpec ---

func TestMutator_EditSpec_SetMinAvailable(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
		e.SetMinAvailable(intstr.FromInt32(2))
		return nil
	})
	require.NoError(t, m.Apply())
	require.NotNil(t, p.Spec.MinAvailable)
	assert.Equal(t, intstr.FromInt32(2), *p.Spec.MinAvailable)
}

func TestMutator_EditSpec_SetMaxUnavailable(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
		e.SetMaxUnavailable(intstr.FromString("25%"))
		return nil
	})
	require.NoError(t, m.Apply())
	require.NotNil(t, p.Spec.MaxUnavailable)
	assert.Equal(t, intstr.FromString("25%"), *p.Spec.MaxUnavailable)
}

func TestMutator_EditSpec_SetSelector(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "web"},
	}
	m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
		e.SetSelector(selector)
		return nil
	})
	require.NoError(t, m.Apply())
	require.NotNil(t, p.Spec.Selector)
	assert.Equal(t, selector, p.Spec.Selector)
}

func TestMutator_EditSpec_SetUnhealthyPodEvictionPolicy(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
		e.SetUnhealthyPodEvictionPolicy(policyv1.AlwaysAllow)
		return nil
	})
	require.NoError(t, m.Apply())
	require.NotNil(t, p.Spec.UnhealthyPodEvictionPolicy)
	assert.Equal(t, policyv1.AlwaysAllow, *p.Spec.UnhealthyPodEvictionPolicy)
}

func TestMutator_EditSpec_RawAccess(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
		val := intstr.FromInt32(3)
		e.Raw().MinAvailable = &val
		return nil
	})
	require.NoError(t, m.Apply())
	require.NotNil(t, p.Spec.MinAvailable)
	assert.Equal(t, intstr.FromInt32(3), *p.Spec.MinAvailable)
}

func TestMutator_EditSpec_Nil(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditSpec(nil)
	assert.NoError(t, m.Apply())
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before spec edits.
	// The spec edit reads a label set by the metadata edit to prove ordering.
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
		// Branch on the label set by the metadata edit: if metadata ran first
		// the label exists and we set MinAvailable to 1; otherwise we set it to 99.
		if p.Labels["order"] == "metadata-first" {
			e.SetMinAvailable(intstr.FromInt32(1))
		} else {
			e.SetMinAvailable(intstr.FromInt32(99))
		}
		return nil
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "metadata-first")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "metadata-first", p.Labels["order"])
	require.NotNil(t, p.Spec.MinAvailable)
	// MinAvailable == 1 proves metadata edits ran before spec edits.
	// If ordering regressed, MinAvailable would be 99.
	assert.Equal(t, intstr.FromInt32(1), *p.Spec.MinAvailable)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("feature1", "on")
		return nil
	})
	m.BeginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("feature2", "on")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "on", p.Labels["feature1"])
	assert.Equal(t, "on", p.Labels["feature2"])
}

func TestMutator_EditSpec_ErrorPropagated(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditSpec(func(_ *editors.PodDisruptionBudgetSpecEditor) error {
		return assert.AnError
	})
	err := m.Apply()
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestMutator_EditObjectMetadata_ErrorPropagated(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		return assert.AnError
	})
	err := m.Apply()
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

// --- Constructor and feature plan invariants ---

func TestNewMutator_InitializesNoPlan(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)

	assert.Empty(t, m.plans, "NewMutator must not create any plans")
	assert.Nil(t, m.active, "active plan must not be set")
}

func TestBeginFeature_AddsExactlyOnePlan(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)

	m.BeginFeature()
	require.Len(t, m.plans, 1, "BeginFeature must add exactly one plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the new plan")

	m.BeginFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestBeginFeature_IsolatesFeaturePlans(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)

	// Record a mutation in the first feature plan
	m.BeginFeature()
	m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
		e.SetMinAvailable(intstr.FromInt32(1))
		return nil
	})

	// Start a new feature and record a different mutation
	m.BeginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("f1", "on")
		return nil
	})

	// The first plan should have exactly one spec edit and no metadata edits
	assert.Len(t, m.plans[0].specEdits, 1, "first plan should have one spec edit")
	assert.Empty(t, m.plans[0].metadataEdits, "first plan should have no metadata edits")
	// The second plan should have exactly one metadata edit and no spec edits
	assert.Len(t, m.plans[1].metadataEdits, 1, "second plan should have one metadata edit")
	assert.Empty(t, m.plans[1].specEdits, "second plan should have no spec edits")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.BeginFeature()
	m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
		e.SetMinAvailable(intstr.FromInt32(1))
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Len(t, m.plans, 1, "no extra plans should be created during Apply")
	require.NotNil(t, p.Spec.MinAvailable)
	assert.Equal(t, intstr.FromInt32(1), *p.Spec.MinAvailable)
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
