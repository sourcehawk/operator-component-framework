package pvc

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

func newValidPVC() *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "test-ns",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidPVC()).Build()
	require.NoError(t, err)
	assert.Equal(t, "v1/PersistentVolumeClaim/test-ns/test-pvc", res.Identity())
}

func TestResource_Object(t *testing.T) {
	pvc := newValidPVC()
	res, err := NewBuilder(pvc).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*corev1.PersistentVolumeClaim)
	require.True(t, ok)
	assert.Equal(t, pvc.Name, got.Name)
	assert.Equal(t, pvc.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-pvc", pvc.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidPVC()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.PersistentVolumeClaim)
	assert.Equal(t, resource.MustParse("10Gi"), got.Spec.Resources.Requests[corev1.ResourceStorage])
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidPVC()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "increase-storage",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetStorageRequest(resource.MustParse("50Gi"))
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.PersistentVolumeClaim)
	assert.Equal(t, resource.MustParse("50Gi"), got.Spec.Resources.Requests[corev1.ResourceStorage])
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidPVC()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetStorageRequest(resource.MustParse("20Gi"))
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetStorageRequest(resource.MustParse("30Gi"))
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.PersistentVolumeClaim)
	assert.Equal(t, resource.MustParse("30Gi"), got.Spec.Resources.Requests[corev1.ResourceStorage])
}

func TestResource_ConvergingStatus(t *testing.T) {
	tests := []struct {
		name           string
		phase          corev1.PersistentVolumeClaimPhase
		expectedStatus concepts.OperationalStatus
	}{
		{"Bound", corev1.ClaimBound, concepts.OperationalStatusOperational},
		{"Pending", corev1.ClaimPending, concepts.OperationalStatusPending},
		{"Lost", corev1.ClaimLost, concepts.OperationalStatusFailing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pvc := newValidPVC()
			pvc.Status.Phase = tt.phase
			res, err := NewBuilder(pvc).Build()
			require.NoError(t, err)

			status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, status.Status)
		})
	}
}

func TestResource_DeleteOnSuspend(t *testing.T) {
	res, err := NewBuilder(newValidPVC()).Build()
	require.NoError(t, err)
	assert.False(t, res.DeleteOnSuspend())
}

func TestResource_Suspend_And_SuspensionStatus(t *testing.T) {
	res, err := NewBuilder(newValidPVC()).Build()
	require.NoError(t, err)

	err = res.Suspend()
	require.NoError(t, err)

	status, err := res.SuspensionStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
}
