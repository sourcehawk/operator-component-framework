package resources_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestDeploymentMutations verifies the factory registers the Version mutation
// and that it fires at every version. The version gate has no constraint, so
// the mutation always applies and rewrites the image tag.
func TestDeploymentMutations(t *testing.T) {
	owner := testOwner("1.0.0")
	res, err := resources.NewDeploymentResource(owner)
	require.NoError(t, err)

	inspector, ok := res.(concepts.MutationInspector)
	require.True(t, ok)

	assert.ElementsMatch(t, []string{"Version"}, inspector.RegisteredMutations())

	firing, err := inspector.FiringSet()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Version"}, firing)
}

// TestDeploymentShape verifies the app component's Deployment as built by its
// factory for each version. The baseline carries the latest container layout;
// the Version mutation sets the image tag. Golden files for each version catch
// regressions when the baseline or mutation logic changes.
func TestDeploymentShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	tests := []struct {
		name    string
		version string
		golden  string
	}{
		{
			name:    "v1.0.0",
			version: "1.0.0",
			golden:  "testdata/deployment-v1.yaml",
		},
		{
			name:    "v2.0.0",
			version: "2.0.0",
			golden:  "testdata/deployment-v2.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := testOwner(tt.version)

			res, err := resources.NewDeploymentResource(owner)
			require.NoError(t, err)

			previewer, ok := res.(golden.Previewer)
			require.True(t, ok, "resource does not implement golden.Previewer")
			golden.AssertYAML(t, tt.golden, previewer,
				golden.WithScheme(scheme), golden.Update(*update))
		})
	}
}
