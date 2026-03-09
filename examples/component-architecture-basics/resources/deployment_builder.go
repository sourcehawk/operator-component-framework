// Package resources contains example resource implementations for the component architecture.
package resources

import (
	"errors"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)

// DeploymentBuilder is a helper for configuring and assembling a DeploymentResource.
// It allows for the injection of custom behavior and Feature Mutations,
// enabling a clean and declarative way to define complex resources.
type DeploymentBuilder struct {
	res *DeploymentResource
}

// NewDeploymentBuilder initializes a new builder with a default Deployment object.
func NewDeploymentBuilder(deployment *appsv1.Deployment) *DeploymentBuilder {
	return &DeploymentBuilder{
		res: &DeploymentResource{
			desired: deployment,
		},
	}
}

// WithMutation registers a version-gated Feature Mutation to the resource.
// These are applied sequentially in the resource's Mutate() call.
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
// It returns an error if the desired is nil, or if it lacks a name or namespace.
func (b *DeploymentBuilder) Build() (*DeploymentResource, error) {
	if b.res.desired == nil {
		return nil, errors.New("desired cannot be nil")
	}
	if b.res.desired.Name == "" {
		return nil, errors.New("desired name cannot be empty")
	}
	if b.res.desired.Namespace == "" {
		return nil, errors.New("desired namespace cannot be empty")
	}
	return b.res, nil
}
