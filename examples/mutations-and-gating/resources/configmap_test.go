package resources_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/features"
	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestConfigMapShape verifies the ConfigMap's rendered YAML against golden
// files for each feature combination.
//
// The baseline ConfigMap carries the core server config in its Data field.
// Boolean-gated mutations (MetricsConfigMutation) layer additional sections
// on top. Golden files pin the output so that changes to the baseline or
// mutation logic surface as test failures.
func TestConfigMapShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name    string
		version string
		metrics bool
		golden  string
	}{
		{
			name:    "baseline",
			version: "1.0.0",
			golden:  "testdata/configmap-baseline.yaml",
		},
		{
			name:    "with metrics",
			version: "1.0.0",
			metrics: true,
			golden:  "testdata/configmap-metrics.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := testOwner(tt.version)

			res, err := configmap.NewBuilder(resources.BaseConfigMap(owner)).
				WithMutation(features.MetricsConfigMutation(tt.metrics)).
				Build()
			require.NoError(t, err)

			golden.AssertYAML(t, tt.golden, res, golden.WithScheme(scheme), golden.Update(*update))
		})
	}
}
