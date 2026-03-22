// Package features provides modular feature mutations for the ingress primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/ingress"
	networkingv1 "k8s.io/api/networking/v1"
)

// VersionAnnotation sets a version annotation on the Ingress metadata.
func VersionAnnotation(version string) ingress.Mutation {
	return ingress.Mutation{
		Name:    "Version",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *ingress.Mutator) error {
			m.EditObjectMetadata(func(meta *editors.ObjectMetaEditor) error {
				meta.EnsureAnnotation("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// TLSFeature adds a TLS entry when enabled.
func TLSFeature(enabled bool, appName string) ingress.Mutation {
	return ingress.Mutation{
		Name:    "TLS",
		Feature: feature.NewResourceFeature("any", nil).When(enabled),
		Mutate: func(m *ingress.Mutator) error {
			m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
				e.EnsureTLS(networkingv1.IngressTLS{
					Hosts:      []string{appName + ".example.com"},
					SecretName: appName + "-tls",
				})
				return nil
			})
			m.EditObjectMetadata(func(meta *editors.ObjectMetaEditor) error {
				meta.EnsureAnnotation("cert-manager.io/cluster-issuer", "letsencrypt-prod")
				return nil
			})
			return nil
		},
	}
}
