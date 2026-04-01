package resources_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/features"
	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var update = flag.Bool("update", false, "update golden files")

func testOwner(version string) *sharedapp.ExampleApp {
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{Version: version},
	}
	owner.Name = "my-app"
	owner.Namespace = "default"
	return owner
}

// TestDeploymentShape verifies the Deployment's rendered YAML against golden
// files for each supported version and feature combination.
//
// The baseline object (BaseDeployment) always reflects the latest version's
// desired state (v2: container "app", HTTP + health ports). Legacy mutations
// roll it back for older versions (v1: container "server", HTTP port only).
//
// Mutation registration order mirrors NewDeploymentResource: DebugLogging
// targets ContainerNamed("app") and must come before LegacyContainer which
// renames the container. TracingSidecar uses AllContainers and is
// order-insensitive.
//
// These snapshots catch unintended regressions: if someone updates the
// baseline to accommodate a new v3 layout, the v1 and v2 golden files will
// fail unless the corresponding backward compat mutations still produce the
// correct shape. This ensures that changes to the latest version do not silently
// break the resource shape served to older versions.
func TestDeploymentShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	tests := []struct {
		name    string
		version string
		debug   bool
		tracing bool
		golden  string
	}{
		// v1 cases: BackwardCompatV1Container fires and rolls back the v2
		// baseline to the v1 container layout. If the baseline changes,
		// these golden files catch any v1 regression.
		{
			name:    "v1 legacy container",
			version: "1.9.0",
			golden:  "testdata/deployment-v1.yaml",
		},
		{
			name:    "v1 with tracing",
			version: "1.9.0",
			tracing: true,
			golden:  "testdata/deployment-v1-tracing.yaml",
		},
		{
			name:    "v1 with debug",
			version: "1.9.0",
			debug:   true,
			golden:  "testdata/deployment-v1-debug.yaml",
		},
		{
			name:    "v1 with tracing and debug",
			version: "1.9.0",
			debug:   true,
			tracing: true,
			golden:  "testdata/deployment-v1-tracing-debug.yaml",
		},
		// v2 cases: no legacy mutation fires, so the baseline is rendered
		// as-is. These golden files pin the current latest shape.
		{
			name:    "v2 baseline",
			version: "2.0.0",
			golden:  "testdata/deployment-v2.yaml",
		},
		{
			name:    "v2 with tracing",
			version: "2.0.0",
			tracing: true,
			golden:  "testdata/deployment-v2-tracing.yaml",
		},
		{
			name:    "v2 with debug",
			version: "2.0.0",
			debug:   true,
			golden:  "testdata/deployment-v2-debug.yaml",
		},
		{
			name:    "v2 with tracing and debug",
			version: "2.0.0",
			debug:   true,
			tracing: true,
			golden:  "testdata/deployment-v2-tracing-debug.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := testOwner(tt.version)

			res, err := deployment.NewBuilder(resources.BaseDeployment(owner)).
				WithMutation(features.DebugLoggingMutation(tt.debug)).
				WithMutation(features.BackwardCompatV1Container(tt.version)).
				WithMutation(features.TracingSidecarMutation(tt.tracing)).
				Build()
			require.NoError(t, err)

			golden.AssertYAML(t, tt.golden, res, golden.WithScheme(scheme), golden.Update(*update))
		})
	}
}
