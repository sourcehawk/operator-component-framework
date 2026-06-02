package features_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/features"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

// TestBackwardCompatV1Container verifies that the mutation renames the
// container to "server" and drops the health port for pre-2.0 versions.
func TestBackwardCompatV1Container(t *testing.T) {
	res, err := deployment.NewBuilder(baseDeployment()).
		WithMutation(features.BackwardCompatV1Container("1.9.0")).
		Build()
	require.NoError(t, err)

	previewed, err := res.Preview()
	require.NoError(t, err)

	obj, ok := previewed.(*appsv1.Deployment)
	require.True(t, ok)

	containers := obj.Spec.Template.Spec.Containers
	require.Len(t, containers, 1)
	assert.Equal(t, "server", containers[0].Name)
	assert.Len(t, containers[0].Ports, 1)
	assert.Equal(t, "http", containers[0].Ports[0].Name)
	assert.Equal(t, int32(8080), containers[0].Ports[0].ContainerPort)
}
