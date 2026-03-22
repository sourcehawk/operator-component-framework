package pv

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestPV() *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", pv.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditPVSpec ---

func TestMutator_EditPVSpec(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	m.EditPVSpec(func(e *editors.PVSpecEditor) error {
		e.SetStorageClassName("fast-ssd")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "fast-ssd", pv.Spec.StorageClassName)
}

func TestMutator_EditPVSpec_Nil(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	m.EditPVSpec(nil)
	assert.NoError(t, m.Apply())
}

func TestMutator_EditPVSpec_RawAccess(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	m.EditPVSpec(func(e *editors.PVSpecEditor) error {
		e.Raw().PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, corev1.PersistentVolumeReclaimRetain, pv.Spec.PersistentVolumeReclaimPolicy)
}

// --- Convenience methods ---

func TestMutator_SetStorageClassName(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	m.SetStorageClassName("premium")
	require.NoError(t, m.Apply())
	assert.Equal(t, "premium", pv.Spec.StorageClassName)
}

func TestMutator_SetReclaimPolicy(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	m.SetReclaimPolicy(corev1.PersistentVolumeReclaimDelete)
	require.NoError(t, m.Apply())
	assert.Equal(t, corev1.PersistentVolumeReclaimDelete, pv.Spec.PersistentVolumeReclaimPolicy)
}

func TestMutator_SetMountOptions(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	opts := []string{"hard", "nfsvers=4.1"}
	m.SetMountOptions(opts)
	require.NoError(t, m.Apply())
	assert.Equal(t, opts, pv.Spec.MountOptions)
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before spec edits.
	pv := newTestPV()
	m := NewMutator(pv)
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.SetStorageClassName("standard")
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", pv.Labels["order"])
	assert.Equal(t, "standard", pv.Spec.StorageClassName)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	m.SetStorageClassName("feature1-class")
	m.beginFeature()
	m.SetReclaimPolicy(corev1.PersistentVolumeReclaimRetain)
	require.NoError(t, m.Apply())

	assert.Equal(t, "feature1-class", pv.Spec.StorageClassName)
	assert.Equal(t, corev1.PersistentVolumeReclaimRetain, pv.Spec.PersistentVolumeReclaimPolicy)
}

func TestMutator_LaterFeatureObservesPrior(t *testing.T) {
	pv := newTestPV()
	m := NewMutator(pv)
	m.SetStorageClassName("first")
	m.beginFeature()
	// Second feature should see the storage class set by the first.
	m.EditPVSpec(func(e *editors.PVSpecEditor) error {
		if e.Raw().StorageClassName == "first" {
			e.SetStorageClassName("first-seen")
		}
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "first-seen", pv.Spec.StorageClassName)
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
