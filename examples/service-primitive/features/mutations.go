// Package features provides sample mutations for the service primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/service"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// VersionLabelMutation sets the app.kubernetes.io/version label on the Service.
// It is always enabled.
func VersionLabelMutation(version string) service.Mutation {
	return service.Mutation{
		Name:    "version-label",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *service.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// MetricsPortMutation adds a metrics port to the Service when metrics are enabled.
func MetricsPortMutation(version string, enableMetrics bool) service.Mutation {
	return service.Mutation{
		Name:    "metrics-port",
		Feature: feature.NewResourceFeature(version, nil).When(enableMetrics),
		Mutate: func(m *service.Mutator) error {
			m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
				e.EnsurePort(corev1.ServicePort{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt32(9090),
					Protocol:   corev1.ProtocolTCP,
				})
				return nil
			})
			return nil
		},
	}
}
