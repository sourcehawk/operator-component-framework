package pvc

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMutator_EditObjectMetadata(t *testing.T) {
	t.Parallel()

	t.Run("adds label", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
		}
		m := NewMutator(pvc)
		m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
			e.EnsureLabel("app", "myapp")
			return nil
		})
		require.NoError(t, m.Apply())
		assert.Equal(t, "myapp", pvc.Labels["app"])
	})

	t.Run("nil edit ignored", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{}
		m := NewMutator(pvc)
		m.EditObjectMetadata(nil)
		require.NoError(t, m.Apply())
	})

	t.Run("error propagated", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{}
		m := NewMutator(pvc)
		m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
			return errors.New("metadata error")
		})
		err := m.Apply()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "metadata error")
	})
}

func TestMutator_EditPVCSpec(t *testing.T) {
	t.Parallel()

	t.Run("sets storage request", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{}
		m := NewMutator(pvc)
		m.EditPVCSpec(func(e *editors.PVCSpecEditor) error {
			e.SetStorageRequest(resource.MustParse("10Gi"))
			return nil
		})
		require.NoError(t, m.Apply())
		qty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		assert.True(t, qty.Equal(resource.MustParse("10Gi")))
	})

	t.Run("nil edit ignored", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{}
		m := NewMutator(pvc)
		m.EditPVCSpec(nil)
		require.NoError(t, m.Apply())
	})

	t.Run("error propagated", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{}
		m := NewMutator(pvc)
		m.EditPVCSpec(func(_ *editors.PVCSpecEditor) error {
			return errors.New("spec error")
		})
		err := m.Apply()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec error")
	})
}

func TestMutator_SetStorageRequest(t *testing.T) {
	t.Parallel()
	pvc := &corev1.PersistentVolumeClaim{}
	m := NewMutator(pvc)
	m.SetStorageRequest(resource.MustParse("20Gi"))
	require.NoError(t, m.Apply())
	qty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.True(t, qty.Equal(resource.MustParse("20Gi")))
}

func TestMutator_ExecutionOrder(t *testing.T) {
	t.Parallel()

	t.Run("metadata before spec within same feature", func(t *testing.T) {
		var order []string
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
		}
		m := NewMutator(pvc)
		m.EditPVCSpec(func(_ *editors.PVCSpecEditor) error {
			order = append(order, "spec")
			return nil
		})
		m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
			order = append(order, "metadata")
			return nil
		})
		require.NoError(t, m.Apply())
		assert.Equal(t, []string{"metadata", "spec"}, order)
	})

	t.Run("feature ordering", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
		}
		m := NewMutator(pvc)
		m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
			e.EnsureLabel("feature", "one")
			return nil
		})

		// Simulate second feature
		m.NextFeature()
		m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
			// Later feature sees earlier mutation
			e.EnsureLabel("feature", "two")
			return nil
		})

		require.NoError(t, m.Apply())
		assert.Equal(t, "two", pvc.Labels["feature"])
	})
}

func TestMutator_MultiFeature(t *testing.T) {
	t.Parallel()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	m := NewMutator(pvc)

	// Feature 1: set label and storage
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	m.EditPVCSpec(func(e *editors.PVCSpecEditor) error {
		e.SetStorageRequest(resource.MustParse("5Gi"))
		return nil
	})

	// Feature 2: override storage
	m.NextFeature()
	m.EditPVCSpec(func(e *editors.PVCSpecEditor) error {
		e.SetStorageRequest(resource.MustParse("10Gi"))
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", pvc.Labels["app"])
	qty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.True(t, qty.Equal(resource.MustParse("10Gi")))
}

// --- Constructor and feature plan invariants ---

func TestNewMutator_InitializesOnePlan(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{}
	m := NewMutator(pvc)

	require.Len(t, m.plans, 1, "NewMutator must create exactly one plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the initial plan")
}

func TestNextFeature_AddsExactlyOnePlan(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{}
	m := NewMutator(pvc)

	// Constructor already created one plan.
	require.Len(t, m.plans, 1)

	m.NextFeature()
	require.Len(t, m.plans, 2, "NextFeature must add exactly one plan")
	assert.Equal(t, &m.plans[1], m.active, "active must point to the new plan")
}

func TestNextFeature_IsolatesFeaturePlans(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{}
	m := NewMutator(pvc)

	// Record a mutation in the initial feature plan (created by constructor)
	m.EditPVCSpec(func(e *editors.PVCSpecEditor) error {
		e.SetStorageRequest(resource.MustParse("5Gi"))
		return nil
	})

	// Start a new feature and record a different mutation
	m.NextFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "test")
		return nil
	})

	assert.Len(t, m.plans[0].specEdits, 1, "first plan should have one spec edit")
	assert.Empty(t, m.plans[0].metadataEdits, "first plan should have no metadata edits")
	assert.Len(t, m.plans[1].metadataEdits, 1, "second plan should have one metadata edit")
	assert.Empty(t, m.plans[1].specEdits, "second plan should have no spec edits")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{}
	m := NewMutator(pvc)
	m.SetStorageRequest(resource.MustParse("1Gi"))

	require.NoError(t, m.Apply())
	assert.Len(t, m.plans, 1)
}
