// Package features provides sample mutations for the secret primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/secret"
)

// BaseCredentialsMutation writes the application's core credentials into the Secret.
// It is always enabled.
func BaseCredentialsMutation(version string) secret.Mutation {
	return secret.Mutation{
		Name:    "base-credentials",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *secret.Mutator) error {
			m.SetStringData("username", "app-user")
			m.SetStringData("password", "default-password")
			return nil
		},
	}
}

// VersionLabelMutation sets the app.kubernetes.io/version label on the Secret.
// It is always enabled.
func VersionLabelMutation(version string) secret.Mutation {
	return secret.Mutation{
		Name:    "version-label",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *secret.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// TracingTokenMutation adds an OpenTelemetry tracing auth token to the Secret.
// It is enabled when enableTracing is true.
func TracingTokenMutation(version string, enableTracing bool) secret.Mutation {
	return secret.Mutation{
		Name:    "tracing-token",
		Feature: feature.NewResourceFeature(version, nil).When(enableTracing),
		Mutate: func(m *secret.Mutator) error {
			m.SetStringData("otel-auth-token", "trace-secret-token-abc123")
			return nil
		},
	}
}

// MetricsTokenMutation adds a Prometheus remote-write auth token to the Secret.
// It is enabled when enableMetrics is true.
func MetricsTokenMutation(version string, enableMetrics bool) secret.Mutation {
	return secret.Mutation{
		Name:    "metrics-token",
		Feature: feature.NewResourceFeature(version, nil).When(enableMetrics),
		Mutate: func(m *secret.Mutator) error {
			m.SetStringData("metrics-auth-token", "metrics-secret-token-xyz789")
			return nil
		},
	}
}
