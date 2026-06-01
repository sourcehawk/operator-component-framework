package features_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/features"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

// TestTracingSidecarMutation verifies that the mutation injects a Jaeger
// sidecar and sets JAEGER_AGENT_HOST on all containers.
func TestTracingSidecarMutation(t *testing.T) {
	res, err := deployment.NewBuilder(baseDeployment()).
		WithMutation(features.TracingSidecarMutation(true)).
		Build()
	require.NoError(t, err)

	previewed, err := res.Preview()
	require.NoError(t, err)

	obj, ok := previewed.(*appsv1.Deployment)
	require.True(t, ok)

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
