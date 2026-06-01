package resources_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestConfigMapResource is the resource-level testing layer for the ConfigMap.
// Like the Deployment resource test, it drives the real NewConfigMapResource
// factory from the owner spec and asserts both the firing set (via
// concepts.MutationInspector) and the rendered golden YAML.
//
// The single registered mutation, "metrics-config", is boolean-gated on
// EnableMetrics, so it fires exactly when metrics are enabled.
func TestConfigMapResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name       string
		spec       sharedapp.ExampleAppSpec
		wantFiring []string
		goldenFile string
	}{
		{
			name:       "baseline",
			spec:       sharedapp.ExampleAppSpec{Version: "1.0.0"},
			wantFiring: []string{},
			goldenFile: "testdata/configmap-baseline.yaml",
		},
		{
			name:       "with metrics",
			spec:       sharedapp.ExampleAppSpec{Version: "1.0.0", EnableMetrics: true},
			wantFiring: []string{"metrics-config"},
			goldenFile: "testdata/configmap-metrics.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := testOwner(tt.spec)

			res, err := resources.NewConfigMapResource(owner)
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
