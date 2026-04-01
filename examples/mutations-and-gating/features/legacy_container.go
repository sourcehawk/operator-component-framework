package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	corev1 "k8s.io/api/core/v1"
)

// BackwardCompatV1Container rolls the v2 baseline back to the v1 container
// layout for versions before 2.0.0. In v1, the container was named "server"
// and only exposed the HTTP port.
//
// Backward compatibility mutations are named BackwardCompat<version> so the
// pattern is immediately recognizable. When multiple backward compat mutations
// exist, register the newest first (closest to the baseline) and the oldest
// last. See the guidelines for details.
func BackwardCompatV1Container(version string) deployment.Mutation {
	return deployment.Mutation{
		Name: "BackwardCompatV1Container",
		Feature: feature.NewVersionGate(
			version,
			[]feature.VersionConstraint{MustConstraint("< 2.0.0")},
		),
		Mutate: func(m *deployment.Mutator) error {
			m.EditContainers(selectors.ContainerNamed("app"), func(ce *editors.ContainerEditor) error {
				ce.Raw().Name = "server"
				ce.Raw().Ports = []corev1.ContainerPort{
					{Name: "http", ContainerPort: 8080},
				}
				return nil
			})
			return nil
		},
	}
}
