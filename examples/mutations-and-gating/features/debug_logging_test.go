package features_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/features"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestDebugLoggingMutation verifies that the mutation sets LOG_LEVEL=debug
// on the application container.
func TestDebugLoggingMutation(t *testing.T) {
	base := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app"},
					},
				},
			},
		},
	}

	res, err := deployment.NewBuilder(base).
		WithMutation(features.DebugLoggingMutation(true)).
		Build()
	require.NoError(t, err)

	obj, err := res.PreviewObject()
	require.NoError(t, err)

	containers := obj.Spec.Template.Spec.Containers
	require.Len(t, containers, 1)
	assert.Equal(t, "app", containers[0].Name)

	found := false
	for _, env := range containers[0].Env {
		if env.Name == "LOG_LEVEL" {
			assert.Equal(t, "debug", env.Value)
			found = true
		}
	}
	assert.True(t, found, "LOG_LEVEL not found on container app")
}
