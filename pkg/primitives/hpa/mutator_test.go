package hpa

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestHPA() *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hpa",
			Namespace: "default",
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MaxReplicas: 10,
		},
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", hpa.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

func TestMutator_EditObjectMetadata_Error(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		return errors.New("metadata error")
	})
	assert.EqualError(t, m.Apply(), "metadata error")
}

// --- EditHPASpec ---

func TestMutator_EditHPASpec(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)
	m.EditHPASpec(func(e *editors.HPASpecEditor) error {
		e.SetMaxReplicas(20)
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, int32(20), hpa.Spec.MaxReplicas)
}

func TestMutator_EditHPASpec_Nil(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)
	m.EditHPASpec(nil)
	assert.NoError(t, m.Apply())
}

func TestMutator_EditHPASpec_Error(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)
	m.EditHPASpec(func(_ *editors.HPASpecEditor) error {
		return errors.New("spec error")
	})
	assert.EqualError(t, m.Apply(), "spec error")
}

func TestMutator_EditHPASpec_EnsureMetric(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)
	m.EditHPASpec(func(e *editors.HPASpecEditor) error {
		e.EnsureMetric(autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: int32Ptr(80),
				},
			},
		})
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, hpa.Spec.Metrics, 1)
	assert.Equal(t, corev1.ResourceCPU, hpa.Spec.Metrics[0].Resource.Name)
}

// --- Execution order ---

func TestMutator_ExecutionOrder(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)

	var order []string

	// Register spec edit first, metadata second — metadata must still run first
	m.EditHPASpec(func(e *editors.HPASpecEditor) error {
		order = append(order, "spec")
		e.SetMaxReplicas(5)
		return nil
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		order = append(order, "metadata")
		e.EnsureLabel("test", "value")
		return nil
	})

	require.NoError(t, m.Apply())
	require.Equal(t, []string{"metadata", "spec"}, order)
}

// --- Multiple features ---

func TestMutator_MultipleFeatures(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)

	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("feature", "one")
		return nil
	})

	m.beginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		// Second feature overwrites the label
		e.EnsureLabel("feature", "two")
		return nil
	})
	m.EditHPASpec(func(e *editors.HPASpecEditor) error {
		e.SetMaxReplicas(42)
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Equal(t, "two", hpa.Labels["feature"])
	assert.Equal(t, int32(42), hpa.Spec.MaxReplicas)
}

func int32Ptr(v int32) *int32 { return &v }
