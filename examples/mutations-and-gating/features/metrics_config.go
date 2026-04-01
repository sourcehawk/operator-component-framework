package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
)

// MetricsConfigMutation adds a Prometheus metrics section to app.yaml.
// It is boolean-gated on the enableMetrics flag.
func MetricsConfigMutation(version string, enableMetrics bool) configmap.Mutation {
	return configmap.Mutation{
		Name:    "metrics-config",
		Feature: feature.NewVersionGate(version, nil).When(enableMetrics),
		Mutate: func(m *configmap.Mutator) error {
			m.MergeYAML("app.yaml", `
metrics:
  enabled: true
  port: 9090
  path: /metrics
`)
			return nil
		},
	}
}
