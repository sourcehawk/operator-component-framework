package hpa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	current := &autoscalingv2.HorizontalPodAutoscaler{
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
	desired := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: ptrInt32(2),
			MaxReplicas: 10,
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec and labels are applied
	assert.Equal(t, int32(2), *current.Spec.MinReplicas)
	assert.Equal(t, int32(10), current.Spec.MaxReplicas)
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

func TestDefaultFieldApplicator_PreservesStatus(t *testing.T) {
	current := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 3,
			DesiredReplicas: 5,
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{
					Type:   autoscalingv2.ScalingActive,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	desired := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: ptrInt32(2),
			MaxReplicas: 10,
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec is applied
	assert.Equal(t, int32(2), *current.Spec.MinReplicas)
	assert.Equal(t, int32(10), current.Spec.MaxReplicas)

	// Status from the live object is preserved
	assert.Equal(t, int32(3), current.Status.CurrentReplicas)
	assert.Equal(t, int32(5), current.Status.DesiredReplicas)
	require.Len(t, current.Status.Conditions, 1)
	assert.Equal(t, autoscalingv2.ScalingActive, current.Status.Conditions[0].Type)
	assert.Equal(t, corev1.ConditionTrue, current.Status.Conditions[0].Status)
}

func ptrInt32(v int32) *int32 {
	return &v
}
