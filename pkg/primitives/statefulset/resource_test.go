package statefulset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestDefaultFieldApplicator_Create(t *testing.T) {
	current := &appsv1.StatefulSet{}
	desired := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "my-svc",
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("10Gi"),
							},
						},
					},
				},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	assert.Equal(t, "my-svc", current.Spec.ServiceName)
	require.Len(t, current.Spec.VolumeClaimTemplates, 1)
	assert.Equal(t, "data", current.Spec.VolumeClaimTemplates[0].Name)
}

func TestDefaultFieldApplicator_Update_PreservesVCTs(t *testing.T) {
	current := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "12345",
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "old-svc",
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "live-data"},
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("10Gi"),
							},
						},
					},
				},
			},
		},
	}
	desired := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "new-svc",
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "desired-data"},
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("50Gi"),
							},
						},
					},
				},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	assert.Equal(t, "new-svc", current.Spec.ServiceName)
	// VCTs should be preserved from the live object, not replaced by desired
	require.Len(t, current.Spec.VolumeClaimTemplates, 1)
	assert.Equal(t, "live-data", current.Spec.VolumeClaimTemplates[0].Name)
	qty := current.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
	assert.Equal(t, "10Gi", qty.String())
}

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	current := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			Namespace:       "default",
			ResourceVersion: "12345",
			UID:             "abc-def",
			Generation:      3,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Pod", Name: "other-owner", UID: "other-uid"},
			},
			Finalizers: []string{"finalizer.example.com"},
		},
	}
	desired := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(int32(3)),
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec and labels are applied
	assert.Equal(t, int32(3), *current.Spec.Replicas)
	assert.Equal(t, "test", current.Labels["app"])

	// Server-managed fields are preserved
	assert.Equal(t, "12345", current.ResourceVersion)
	assert.Equal(t, "abc-def", string(current.UID))
	assert.Equal(t, int64(3), current.Generation)

	// Shared-controller fields are preserved
	assert.Len(t, current.OwnerReferences, 1)
	assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)
}
