// Package features provides sample mutations for the secret primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/secret"
)

// BaseCredentialsMutation writes the application's core credentials into the Secret.
// It is always enabled.
//
// NOTE: Real controllers must never hard-code credentials. Source them from
// external secret stores, environment variables, or operator CR fields.
func BaseCredentialsMutation(version string) secret.Mutation {
	return secret.Mutation{
		Name:    "base-credentials",
		Feature: feature.NewVersionGate(version, nil),
		Mutate: func(m *secret.Mutator) error {
			m.SetStringData("username", "REPLACE_ME")
			m.SetStringData("password", "REPLACE_ME")
			return nil
		},
	}
}

// VersionLabelMutation sets the app.kubernetes.io/version label on the Secret.
// It is always enabled.
func VersionLabelMutation(version string) secret.Mutation {
	return secret.Mutation{
		Name:    "version-label",
		Feature: feature.NewVersionGate(version, nil),
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
		Feature: feature.NewVersionGate(version, nil).When(enableTracing),
		Mutate: func(m *secret.Mutator) error {
			m.SetStringData("otel-auth-token", "REPLACE_ME")
			return nil
		},
	}
}

// MetricsTokenMutation adds a Prometheus remote-write auth token to the Secret.
// It is enabled when enableMetrics is true.
func MetricsTokenMutation(version string, enableMetrics bool) secret.Mutation {
	return secret.Mutation{
		Name:    "metrics-token",
		Feature: feature.NewVersionGate(version, nil).When(enableMetrics),
		Mutate: func(m *secret.Mutator) error {
			m.SetStringData("metrics-auth-token", "REPLACE_ME")
			return nil
		},
	}
}
