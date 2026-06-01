package resources_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var update = flag.Bool("update", false, "update golden files")

func testOwner(spec sharedapp.ExampleAppSpec) *sharedapp.ExampleApp {
	owner := &sharedapp.ExampleApp{Spec: spec}
	owner.Name = "my-app"
	owner.Namespace = "default"
	return owner
}

// TestDeploymentResource is the resource-level testing layer for the Deployment.
// It exercises the real NewDeploymentResource factory (not an inline rebuild) and
// asserts two things per case:
//
//  1. Introspection: exactly the expected set of mutations fires for the owner
//     spec, via the concepts.MutationInspector the built resource implements. This
//     pins the gating decisions independently of the rendered bytes, so a gate
//     regression is reported as a firing-set mismatch rather than a golden diff.
//  2. Golden: the rendered YAML matches the snapshot for that version and feature
//     combination, catching shape regressions (e.g. a baseline change that breaks
//     the v1 backward-compat rollback).
//
// The owner spec drives the build: DebugLogging is boolean-gated on
// EnableDebugLogging, Tracing on EnableTracing, and BackwardCompatV1Container is
// version-gated to fire for versions < 2.0.0.
func TestDeploymentResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	tests := []struct {
		name       string
		spec       sharedapp.ExampleAppSpec
		wantFiring []string
		goldenFile string
	}{
		// v1 cases: BackwardCompatV1Container fires (version < 2.0.0) and rolls the
		// v2 baseline back to the v1 container layout.
		{
			name:       "v1 legacy container",
			spec:       sharedapp.ExampleAppSpec{Version: "1.9.0"},
			wantFiring: []string{"BackwardCompatV1Container"},
			goldenFile: "testdata/deployment-v1.yaml",
		},
		{
			name:       "v1 with tracing",
			spec:       sharedapp.ExampleAppSpec{Version: "1.9.0", EnableTracing: true},
			wantFiring: []string{"BackwardCompatV1Container", "Tracing"},
			goldenFile: "testdata/deployment-v1-tracing.yaml",
		},
		{
			name:       "v1 with debug",
			spec:       sharedapp.ExampleAppSpec{Version: "1.9.0", EnableDebugLogging: true},
			wantFiring: []string{"BackwardCompatV1Container", "DebugLogging"},
			goldenFile: "testdata/deployment-v1-debug.yaml",
		},
		{
			name:       "v1 with tracing and debug",
			spec:       sharedapp.ExampleAppSpec{Version: "1.9.0", EnableDebugLogging: true, EnableTracing: true},
			wantFiring: []string{"BackwardCompatV1Container", "DebugLogging", "Tracing"},
			goldenFile: "testdata/deployment-v1-tracing-debug.yaml",
		},
		// v2 cases: BackwardCompatV1Container does not fire, so the baseline renders
		// as the latest shape.
		{
			name:       "v2 baseline",
			spec:       sharedapp.ExampleAppSpec{Version: "2.0.0"},
			wantFiring: []string{},
			goldenFile: "testdata/deployment-v2.yaml",
		},
		{
			name:       "v2 with tracing",
			spec:       sharedapp.ExampleAppSpec{Version: "2.0.0", EnableTracing: true},
			wantFiring: []string{"Tracing"},
			goldenFile: "testdata/deployment-v2-tracing.yaml",
		},
		{
			name:       "v2 with debug",
			spec:       sharedapp.ExampleAppSpec{Version: "2.0.0", EnableDebugLogging: true},
			wantFiring: []string{"DebugLogging"},
			goldenFile: "testdata/deployment-v2-debug.yaml",
		},
		{
			name:       "v2 with tracing and debug",
			spec:       sharedapp.ExampleAppSpec{Version: "2.0.0", EnableDebugLogging: true, EnableTracing: true},
			wantFiring: []string{"DebugLogging", "Tracing"},
			goldenFile: "testdata/deployment-v2-tracing-debug.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := testOwner(tt.spec)

			res, err := resources.NewDeploymentResource(owner)
			require.NoError(t, err)

			inspector, ok := res.(concepts.MutationInspector)
			require.True(t, ok, "resource must implement MutationInspector")
			firing, err := inspector.FiringSet()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantFiring, firing)

			previewer, ok := res.(golden.Previewer)
			require.True(t, ok, "resource must implement golden.Previewer")
			golden.AssertYAML(t, tt.goldenFile, previewer, golden.WithScheme(scheme), golden.Update(*update))
		})
	}
}
