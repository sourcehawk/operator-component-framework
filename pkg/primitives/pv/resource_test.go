package pv

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidPV() *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: "/data"},
			},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidPV()).Build()
	require.NoError(t, err)
	assert.Equal(t, "v1/PersistentVolume/test-pv", res.Identity())
}

func TestResource_Object(t *testing.T) {
	pv := newValidPV()
	res, err := NewBuilder(pv).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*corev1.PersistentVolume)
	require.True(t, ok)
	assert.Equal(t, pv.Name, got.Name)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-pv", pv.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidPV()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.PersistentVolume)
	expectedCapacity := resource.MustParse("10Gi")
	actualCapacity := got.Spec.Capacity[corev1.ResourceStorage]
	assert.True(t, expectedCapacity.Equal(actualCapacity))
	assert.Equal(t, "/data", got.Spec.HostPath.Path)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidPV()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "set-storage-class",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetStorageClassName("fast-ssd")
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.PersistentVolume)
	assert.Equal(t, "fast-ssd", got.Spec.StorageClassName)
	assert.Equal(t, "/data", got.Spec.HostPath.Path)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidPV()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetStorageClassName("a")
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetStorageClassName("b")
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.PersistentVolume)
	assert.Equal(t, "b", got.Spec.StorageClassName)
}

func TestResource_ConvergingStatus(t *testing.T) {
	desired := newValidPV()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	status, err := res.ConvergingStatus(concepts.ConvergingOperationNone)
	require.NoError(t, err)
	// Default handler on a PV with no phase set returns OperationPending.
	assert.Equal(t, concepts.OperationalStatusPending, status.Status)
}
