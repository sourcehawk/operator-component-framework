// Package resources builds the version-gated StatefulSet for the version-matrix example.
package resources

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/sourcehawk/operator-component-framework/examples/version-matrix/app"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// semverConstraint adapts a Masterminds/semver constraint to feature.VersionConstraint.
type semverConstraint struct {
	c *semver.Constraints
}

// mustConstraint parses a semver constraint expression or panics. It is only used
// at package init with literal expressions, so a parse failure is a programming
// error rather than a runtime condition.
func mustConstraint(expr string) feature.VersionConstraint {
	c, err := semver.NewConstraint(expr)
	if err != nil {
		panic(err)
	}
	return &semverConstraint{c: c}
}

// Enabled reports whether the constraint is satisfied for the given version.
func (s *semverConstraint) Enabled(version string) (bool, error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return false, fmt.Errorf("parse version %q: %w", version, err)
	}
	return s.c.Check(v), nil
}

// BaseStatefulSet returns the desired-state StatefulSet for the given owner before
// any version-gated mutation is applied.
func BaseStatefulSet(owner *app.ExampleApp) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-db",
			Namespace: owner.Namespace,
			Labels:    map[string]string{"app": owner.Name},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: owner.Name,
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": owner.Name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": owner.Name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "db"}},
				},
			},
		},
	}
}

// ContainerImageMutation pins the container image to the owner's version. It has no
// gate, so it fires at every version and anchors the always-on part of the firing
// set.
func ContainerImageMutation(owner *app.ExampleApp) statefulset.Mutation {
	return statefulset.Mutation{
		Name: "ContainerImage",
		Mutate: func(m *statefulset.Mutator) error {
			m.EditContainers(selectors.AllContainers(), func(c *editors.ContainerEditor) error {
				c.Raw().Image = fmt.Sprintf("example/db:%s", owner.Spec.Version)
				return nil
			})
			return nil
		},
	}
}

// ClusterEnvPre89Mutation sets the pre-8.9 cluster-coordination environment
// variable. It fires only for versions below 8.9.0, where the unified protocol is
// not yet available.
func ClusterEnvPre89Mutation(version string) statefulset.Mutation {
	return statefulset.Mutation{
		Name:    "ClusterEnv/Pre89",
		Feature: feature.NewVersionGate(version, []feature.VersionConstraint{mustConstraint("< 8.9.0")}),
		Mutate: func(m *statefulset.Mutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CLUSTER_DISCOVERY", Value: "legacy-gossip"})
			return nil
		},
	}
}

// ClusterEnvUnified89Mutation sets the unified cluster-coordination environment
// variable introduced in 8.9.0. It fires only for versions at or above 8.9.0.
func ClusterEnvUnified89Mutation(version string) statefulset.Mutation {
	return statefulset.Mutation{
		Name:    "ClusterEnv/Unified89",
		Feature: feature.NewVersionGate(version, []feature.VersionConstraint{mustConstraint(">= 8.9.0")}),
		Mutate: func(m *statefulset.Mutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CLUSTER_DISCOVERY", Value: "unified-raft"})
			return nil
		},
	}
}

// NewStatefulSetResource builds the StatefulSet for the owner with its version-gated
// mutations registered. The owner's Spec.Version drives every gate, so the same
// build wired through goldengen produces a distinct golden per gating regime.
func NewStatefulSetResource(owner *app.ExampleApp) (*statefulset.Resource, error) {
	return statefulset.NewBuilder(BaseStatefulSet(owner)).
		WithMutation(ContainerImageMutation(owner)).
		WithMutation(ClusterEnvPre89Mutation(owner.Spec.Version)).
		WithMutation(ClusterEnvUnified89Mutation(owner.Spec.Version)).
		Build()
}
