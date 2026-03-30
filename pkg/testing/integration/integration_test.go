package integration_test

import (
	"flag"
	"fmt"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

var update = flag.Bool("update", false, "update golden files")

// semverConstraint implements feature.VersionConstraint using Masterminds/semver.
type semverConstraint struct {
	c *semver.Constraints
}

func mustConstraint(expr string) *semverConstraint {
	c, err := semver.NewConstraint(expr)
	if err != nil {
		panic(err)
	}
	return &semverConstraint{c: c}
}

func (s *semverConstraint) Enabled(version string) (bool, error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return false, err
	}
	return s.c.Check(v), nil
}

// buildDeployment constructs a deployment resource using the real primitive
// builder and mutation system. The baseline represents the latest desired
// state (v2.0+) and a legacy mutation rolls it back for older versions.
func buildDeployment(version string, tracing bool) (*deployment.Resource, error) {
	base := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app-web",
			Namespace: "production",
			Labels: map[string]string{
				"app":                          "my-app",
				"app.kubernetes.io/managed-by": "my-operator",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](3),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "my-app"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "my-app"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: fmt.Sprintf("my-app:%s", version),
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 8080},
								{Name: "health", ContainerPort: 8081},
							},
						},
					},
				},
			},
		},
	}

	builder := deployment.NewBuilder(base)
	builder.WithMutation(versionLabel(version))
	builder.WithMutation(legacyContainerName(version))
	builder.WithMutation(tracingFeature(version, tracing))

	return builder.Build()
}

func versionLabel(version string) deployment.Mutation {
	return deployment.Mutation{
		Name:    "version-label",
		Feature: feature.NewVersionGate(version, nil),
		Mutate: func(m *deployment.Mutator) error {
			m.EditObjectMetadata(func(meta *editors.ObjectMetaEditor) error {
				meta.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// legacyContainerName rolls the container name back to "server" and removes
// the health port for versions before 2.0.0.
func legacyContainerName(version string) deployment.Mutation {
	return deployment.Mutation{
		Name: "legacy-container-name",
		Feature: feature.NewVersionGate(
			version,
			[]feature.VersionConstraint{mustConstraint("< 2.0.0")},
		),
		Mutate: func(m *deployment.Mutator) error {
			m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
				e.Raw().Name = "server"
				e.Raw().Ports = []corev1.ContainerPort{
					{Name: "http", ContainerPort: 8080},
				}
				return nil
			})
			return nil
		},
	}
}

// tracingFeature adds a tracing sidecar when enabled.
func tracingFeature(version string, enabled bool) deployment.Mutation {
	return deployment.Mutation{
		Name:    "tracing",
		Feature: feature.NewVersionGate(version, nil).When(enabled),
		Mutate: func(m *deployment.Mutator) error {
			m.EnsureContainer(corev1.Container{
				Name:  "jaeger-agent",
				Image: "jaegertracing/jaeger-agent:1.28",
			})
			return nil
		},
	}
}

func TestDeploymentShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	tests := []struct {
		name    string
		version string
		tracing bool
		golden  string
	}{
		{
			name:    "v1.9 baseline",
			version: "1.9.0",
			golden:  "testdata/deployment-v1.9.0.yaml",
		},
		{
			name:    "v1.9 with tracing",
			version: "1.9.0",
			tracing: true,
			golden:  "testdata/deployment-v1.9.0-tracing.yaml",
		},
		{
			name:    "v2.0 baseline",
			version: "2.0.0",
			golden:  "testdata/deployment-v2.0.0.yaml",
		},
		{
			name:    "v2.0 with tracing",
			version: "2.0.0",
			tracing: true,
			golden:  "testdata/deployment-v2.0.0-tracing.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := buildDeployment(tt.version, tt.tracing)
			require.NoError(t, err)

			golden.AssertYAML(t, tt.golden, res, golden.WithScheme(scheme), golden.Update(*update))
		})
	}
}
