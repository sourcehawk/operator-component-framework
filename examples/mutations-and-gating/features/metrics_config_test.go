package features_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/features"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// TestMetricsConfigMutation verifies that the mutation merges a Prometheus
// metrics section into app.yaml.
func TestMetricsConfigMutation(t *testing.T) {
	res, err := configmap.NewBuilder(baseConfigMap()).
		WithMutation(features.MetricsConfigMutation(true)).
		Build()
	require.NoError(t, err)

	previewed, err := res.Preview()
	require.NoError(t, err)

	obj, ok := previewed.(*corev1.ConfigMap)
	require.True(t, ok)

	yaml := obj.Data["app.yaml"]
	assert.Contains(t, yaml, "metrics:")
	assert.Contains(t, yaml, "port: 9090")
	assert.Contains(t, yaml, "path: /metrics")
	assert.Contains(t, yaml, "server:")
}
