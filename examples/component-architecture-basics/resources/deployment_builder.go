// Package resources contains example resource implementations for the component architecture.
package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)

// DeploymentBuilder is a helper for configuring and assembling a DeploymentResource.
// It allows for the injection of custom behavior and Feature Mutations,
// enabling a clean and declarative way to define complex resources.
type DeploymentBuilder struct {
	res *DeploymentResource
}

// NewDeploymentBuilder initializes a new builder with basic resource metadata.
func NewDeploymentBuilder(name, namespace string) *DeploymentBuilder {
	return &DeploymentBuilder{
		res: &DeploymentResource{
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
		},
	}
}

// WithMutation registers a version-gated Feature Mutation to the resource.
// These are applied sequentially in the resource's SetMutable() call.
func (b *DeploymentBuilder) WithMutation(m feature.Mutation[*DeploymentResourceMutator]) *DeploymentBuilder {
	b.res.mutations = append(b.res.mutations, m)
	return b
}

// WithCustomConvergeStatus overrides the default readiness logic for the resource.
// This is part of implementing the Alive interface for the Component Framework.
func (b *DeploymentBuilder) WithCustomConvergeStatus(
	handler func(component.ConvergingOperation, *appsv1.Deployment) (component.ConvergingStatusWithReason, error),
) *DeploymentBuilder {
	if handler == nil {
		return b
	}
	b.res.convergeStatusHandler = handler
	return b
}

// WithCustomGraceStatus overrides the default health reporting for degraded resources.
func (b *DeploymentBuilder) WithCustomGraceStatus(
	handler func(*appsv1.Deployment) (component.GraceStatusWithReason, error),
) *DeploymentBuilder {
	if handler == nil {
		return b
	}
	b.res.graceStatusHandler = handler
	return b
}

// WithCustomSuspendStatus overrides the default suspension status reporting.
func (b *DeploymentBuilder) WithCustomSuspendStatus(
	handler func(*appsv1.Deployment) (component.SuspensionStatusWithReason, error),
) *DeploymentBuilder {
	if handler == nil {
		return b
	}
	b.res.suspendStatusHandler = handler
	return b
}

// WithCustomSuspendMutation overrides how the resource is suspended (e.g., scaling to 0).
func (b *DeploymentBuilder) WithCustomSuspendMutation(
	handler func(*appsv1.Deployment) error,
) *DeploymentBuilder {
	if handler == nil {
		return b
	}
	b.res.suspendMutationHandler = handler
	return b
}

// WithCustomSuspendDeletionDecision overrides whether a resource is deleted on suspension.
func (b *DeploymentBuilder) WithCustomSuspendDeletionDecision(
	handler func(*appsv1.Deployment) bool,
) *DeploymentBuilder {
	if handler == nil {
		return b
	}
	b.res.suspendDeletionDecisionHandler = handler
	return b
}

// WithDataExtractor registers a function to pull information from the resource during reconciliation.
// This implements the DataExtractable interface.
// The extractor is intentionally a pass by value method to prevent mutations for read-only data extraction.
func (b *DeploymentBuilder) WithDataExtractor(
	extractor func(appsv1.Deployment) error,
) *DeploymentBuilder {
	if extractor == nil {
		return b
	}
	b.res.dataExtractors = append(b.res.dataExtractors, extractor)
	return b
}

// Build finalizes and returns the configured DeploymentResource.
func (b *DeploymentBuilder) Build() *DeploymentResource {
	return b.res
}
