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
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditSpec ---

func TestMutator_EditSpec_SetMinAvailable(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
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
	m.EditSpec(nil)
	assert.NoError(t, m.Apply())
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before spec edits.
	p := newTestPDB()
	m := NewMutator(p)
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
		e.SetMinAvailable(intstr.FromInt32(1))
		return nil
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", p.Labels["order"])
	require.NotNil(t, p.Spec.MinAvailable)
	assert.Equal(t, intstr.FromInt32(1), *p.Spec.MinAvailable)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	p := newTestPDB()
	m := NewMutator(p)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("feature1", "on")
		return nil
	})
	m.beginFeature()
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
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		return assert.AnError
	})
	err := m.Apply()
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
