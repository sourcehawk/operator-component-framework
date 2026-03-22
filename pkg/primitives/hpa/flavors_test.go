package hpa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPreserveCurrentLabels(t *testing.T) {
	t.Parallel()

	applied := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app": "desired-value",
			},
		},
	}
	current := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app":      "current-value",
				"external": "preserved",
			},
		},
	}

	err := PreserveCurrentLabels(applied, current, nil)
	require.NoError(t, err)
	assert.Equal(t, "desired-value", applied.Labels["app"])
	assert.Equal(t, "preserved", applied.Labels["external"])
}

func TestPreserveCurrentAnnotations(t *testing.T) {
	t.Parallel()

	applied := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"config": "desired",
			},
		},
	}
	current := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"config":   "current",
				"external": "preserved",
			},
		},
	}

	err := PreserveCurrentAnnotations(applied, current, nil)
	require.NoError(t, err)
	assert.Equal(t, "desired", applied.Annotations["config"])
	assert.Equal(t, "preserved", applied.Annotations["external"])
}

func TestFlavors_RunInRegistrationOrder(t *testing.T) {
	t.Parallel()

	base := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hpa",
			Namespace: "default",
		},
	}

	res, err := NewBuilder(base).
		WithFieldApplicationFlavor(PreserveCurrentLabels).
		WithFieldApplicationFlavor(PreserveCurrentAnnotations).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.FieldFlavors, 2)
}
