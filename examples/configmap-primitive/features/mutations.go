// Package features provides sample mutations for the configmap primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
)

// BaseConfigMutation writes the application's core server settings into app.yaml.
// It is always enabled.
func BaseConfigMutation(version string) configmap.Mutation {
	return configmap.Mutation{
		Name:    "base-config",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *configmap.Mutator) error {
			m.MergeYAML("app.yaml", `
server:
  port: 8080
  timeout: 30s
`)
			return nil
		},
	}
}

// VersionLabelMutation sets the app.kubernetes.io/version label on the ConfigMap
// and records the version in app.yaml. It is always enabled.
func VersionLabelMutation(version string) configmap.Mutation {
	return configmap.Mutation{
		Name:    "version-label",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *configmap.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			m.MergeYAML("app.yaml", "app:\n  version: "+version+"\n")
			return nil
		},
	}
}

// TracingConfigMutation adds an OpenTelemetry tracing section to app.yaml.
// It is enabled when enableTracing is true.
func TracingConfigMutation(version string, enableTracing bool) configmap.Mutation {
	return configmap.Mutation{
		Name:    "tracing-config",
		Feature: feature.NewResourceFeature(version, nil).When(enableTracing),
		Mutate: func(m *configmap.Mutator) error {
			m.MergeYAML("app.yaml", `
tracing:
  enabled: true
  endpoint: otel-collector:4317
  sampling_rate: 0.1
`)
			return nil
		},
	}
}

// MetricsConfigMutation adds a Prometheus metrics section to app.yaml.
// It is enabled when enableMetrics is true.
func MetricsConfigMutation(version string, enableMetrics bool) configmap.Mutation {
	return configmap.Mutation{
		Name:    "metrics-config",
		Feature: feature.NewResourceFeature(version, nil).When(enableMetrics),
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
