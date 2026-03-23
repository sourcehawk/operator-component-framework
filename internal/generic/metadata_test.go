package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func TestPreserveServerManagedFields(t *testing.T) {
	timestamp := metav1.Now()
	deletionTimestamp := metav1.Now()

	src := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion:            "12345",
			UID:                        types.UID("abc-def"),
			Generation:                 3,
			CreationTimestamp:          timestamp,
			DeletionTimestamp:          &deletionTimestamp,
			DeletionGracePeriodSeconds: ptr.To(int64(30)),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "test-manager", Operation: metav1.ManagedFieldsOperationApply},
			},
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Pod", Name: "owner", UID: "owner-uid"},
			},
			Finalizers: []string{"finalizer.example.com"},
		},
	}
	src.SetSelfLink("/api/v1/namespaces/test/configmaps/test-cm")

	dst := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "test-ns",
			Labels:    map[string]string{"app": "test"},
		},
		Data: map[string]string{"key": "value"},
	}

	PreserveServerManagedFields(dst, src)

	// Server-managed fields are restored
	assert.Equal(t, "12345", dst.ResourceVersion)
	assert.Equal(t, types.UID("abc-def"), dst.UID)
	assert.Equal(t, int64(3), dst.Generation)
	assert.Equal(t, timestamp, dst.CreationTimestamp)
	assert.Equal(t, &deletionTimestamp, dst.DeletionTimestamp)
	assert.Equal(t, ptr.To(int64(30)), dst.DeletionGracePeriodSeconds)
	assert.Equal(t, "/api/v1/namespaces/test/configmaps/test-cm", dst.GetSelfLink())
	assert.Len(t, dst.ManagedFields, 1)
	assert.Equal(t, "test-manager", dst.ManagedFields[0].Manager)

	// Shared-controller fields are restored
	assert.Len(t, dst.OwnerReferences, 1)
	assert.Equal(t, "owner", dst.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, dst.Finalizers)

	// Non-metadata fields are not affected
	assert.Equal(t, "test-cm", dst.Name)
	assert.Equal(t, "test-ns", dst.Namespace)
	assert.Equal(t, "test", dst.Labels["app"])
	assert.Equal(t, "value", dst.Data["key"])
}

func TestPreserveServerManagedFields_ZeroValues(t *testing.T) {
	src := &corev1.ConfigMap{}
	dst := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}

	PreserveServerManagedFields(dst, src)

	assert.Empty(t, dst.ResourceVersion)
	assert.Empty(t, dst.UID)
	assert.Zero(t, dst.Generation)
	assert.Nil(t, dst.DeletionTimestamp)
	assert.Nil(t, dst.DeletionGracePeriodSeconds)
	assert.Nil(t, dst.ManagedFields)
	assert.Nil(t, dst.OwnerReferences)
	assert.Nil(t, dst.Finalizers)
	assert.Equal(t, "test", dst.Name)
}

func TestPreserveStatus(t *testing.T) {
	t.Run("copies status from src to dst", func(t *testing.T) {
		src := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				ReadyReplicas:     3,
				Replicas:          5,
				AvailableReplicas: 3,
			},
		}
		dst := &appsv1.Deployment{}

		PreserveStatus(dst, src)

		assert.Equal(t, int32(3), dst.Status.ReadyReplicas)
		assert.Equal(t, int32(5), dst.Status.Replicas)
		assert.Equal(t, int32(3), dst.Status.AvailableReplicas)
	})

	t.Run("overwrites existing status on dst", func(t *testing.T) {
		src := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 5,
			},
		}
		dst := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 1,
			},
		}

		PreserveStatus(dst, src)

		assert.Equal(t, int32(5), dst.Status.ReadyReplicas)
	})

	t.Run("zero status from src clears dst status", func(t *testing.T) {
		src := &appsv1.Deployment{}
		dst := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 3,
			},
		}

		PreserveStatus(dst, src)

		assert.Equal(t, int32(0), dst.Status.ReadyReplicas)
	})

	t.Run("no-op for objects without Status field", func(t *testing.T) {
		src := &corev1.ConfigMap{Data: map[string]string{"key": "val"}}
		dst := &corev1.ConfigMap{}

		// Should not panic.
		PreserveStatus(dst, src)

		assert.Nil(t, dst.Data)
	})

	t.Run("nil dst is a no-op", func(_ *testing.T) {
		var dst *appsv1.Deployment
		src := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{ReadyReplicas: 3},
		}
		// Should not panic.
		PreserveStatus(dst, src)
	})

	t.Run("nil src is a no-op", func(_ *testing.T) {
		dst := &appsv1.Deployment{}
		var src *appsv1.Deployment
		// Should not panic.
		PreserveStatus(dst, src)
	})

	t.Run("does not affect non-status fields", func(t *testing.T) {
		src := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "src-name"},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 3,
			},
		}
		dst := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dst-name"},
		}

		PreserveStatus(dst, src)

		assert.Equal(t, "dst-name", dst.Name)
		assert.Equal(t, int32(3), dst.Status.ReadyReplicas)
	})
}
