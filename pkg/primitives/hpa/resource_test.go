package hpa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResource_ObjectAndMutate(t *testing.T) {
	base := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hpa",
			Namespace: "default",
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: ptrInt32(2),
			MaxReplicas: 10,
		},
	}

	res, err := NewBuilder(base).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	assert.Equal(t, int32(2), *got.Spec.MinReplicas)
	assert.Equal(t, int32(10), got.Spec.MaxReplicas)
}

func ptrInt32(v int32) *int32 {
	return &v
}
