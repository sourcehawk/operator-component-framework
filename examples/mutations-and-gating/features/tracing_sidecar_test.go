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

// TestTracingSidecarMutation verifies that the mutation injects a Jaeger
// sidecar and sets JAEGER_AGENT_HOST on all containers.
func TestTracingSidecarMutation(t *testing.T) {
	base := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "web"},
					},
				},
			},
		},
	}

	res, err := deployment.NewBuilder(base).
		WithMutation(features.TracingSidecarMutation(true)).
		Build()
	require.NoError(t, err)

	obj, err := res.PreviewObject()
	require.NoError(t, err)

	containers := obj.Spec.Template.Spec.Containers
	require.Len(t, containers, 2)

	sidecar := containers[1]
	assert.Equal(t, "jaeger-agent", sidecar.Name)
	assert.Equal(t, "jaegertracing/jaeger-agent:1.28", sidecar.Image)

	for _, c := range containers {
		found := false
		for _, env := range c.Env {
			if env.Name == "JAEGER_AGENT_HOST" {
				assert.Equal(t, "localhost", env.Value)
				found = true
			}
		}
		assert.True(t, found, "JAEGER_AGENT_HOST not found on container %s", c.Name)
	}
}
