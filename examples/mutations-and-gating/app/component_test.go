package app_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/app"
	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// update is this package's own -update flag. The resources package declares its
// own; the two live in separate test binaries, so there is no conflict.
var update = flag.Bool("update", false, "update golden files")

func testOwner(spec sharedapp.ExampleAppSpec) *app.ExampleApp {
	owner := &app.ExampleApp{Spec: spec}
	owner.Name = "my-app"
	owner.Namespace = "default"
	return owner
}

// TestBuildComponent is the component-level testing layer: it builds the whole
// component the controller reconciles (via the shared app.BuildComponent) and
// goldens the multi-document YAML of every resource it would apply.
//
// Unlike the resource-level tests, which pin one resource at a time, this asserts
// the composed desired state: the Deployment and ConfigMap rendered together, in
// the order the component applies them. It catches regressions in how the
// resources are wired into the component, not just in each resource's shape.
func TestBuildComponent(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name   string
		spec   sharedapp.ExampleAppSpec
		golden string
	}{
		{
			name: "v2 all features on",
			spec: sharedapp.ExampleAppSpec{
				Version:            "2.0.0",
				EnableDebugLogging: true,
				EnableTracing:      true,
				EnableMetrics:      true,
			},
			golden: "testdata/component-v2-all.yaml",
		},
		{
			name:   "v1 minimal",
			spec:   sharedapp.ExampleAppSpec{Version: "1.9.0"},
			golden: "testdata/component-v1-minimal.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := testOwner(tt.spec)

			comp, err := app.BuildComponent(owner, resources.NewDeploymentResource, resources.NewConfigMapResource)
			require.NoError(t, err)

			golden.AssertComponentYAML(t, tt.golden, comp, golden.WithScheme(scheme), golden.Update(*update))
		})
	}
}
