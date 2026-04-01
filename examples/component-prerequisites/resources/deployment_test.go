package resources_test

import (
	"fmt"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestDeploymentShape verifies the app component's Deployment for each version.
// The baseline carries the latest container layout; the version mutation sets
// the image tag. Golden files for each version catch regressions when the
// baseline or mutation logic changes.
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

			res, err := deployment.NewBuilder(resources.BaseDeployment(owner)).
				WithMutation(deployment.Mutation{
					Name:    "Version",
					Feature: feature.NewVersionGate(tt.version, nil),
					Mutate: func(m *deployment.Mutator) error {
						m.EditContainers(selectors.ContainerNamed("app"), func(ce *editors.ContainerEditor) error {
							ce.Raw().Image = fmt.Sprintf("my-app:%s", tt.version)
							return nil
						})
						return nil
					},
				}).
				Build()
			require.NoError(t, err)

			golden.AssertYAML(t, tt.golden, res, golden.WithScheme(scheme), golden.Update(*update))
		})
	}
}
