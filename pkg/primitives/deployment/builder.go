// Package deployment provides a builder and resource for managing Kubernetes Deployments.
package deployment

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	appsv1 "k8s.io/api/apps/v1"
)

// Builder is a configuration helper for creating and customizing a Deployment Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. This builder ensures that the resulting Resource is
// properly initialized and validated before use in a reconciliation loop.
type Builder struct {
	base *generic.WorkloadBuilder[*appsv1.Deployment, *Mutator]
}

// NewBuilder initializes a new Builder with the provided Deployment object.
//
// The Deployment object passed here serves as the "desired base state". During
// reconciliation, the Resource will attempt to make the cluster's state match
// this base state, modified by any registered mutations.
//
// The provided deployment must have at least a Name and Namespace set, which
// is validated during the Build() call.
func NewBuilder(deployment *appsv1.Deployment) *Builder {
	identityFunc := func(d *appsv1.Deployment) string {
		return fmt.Sprintf("apps/v1/Deployment/%s/%s", d.Namespace, d.Name)
	}

	base := generic.NewWorkloadBuilder[*appsv1.Deployment, *Mutator](
		deployment,
		identityFunc,
		DefaultFieldApplicator,
		NewMutator,
	)

	base.
		WithCustomConvergeStatus(DefaultConvergingStatusHandler).
		WithCustomGraceStatus(DefaultGraceStatusHandler).
		WithCustomSuspendStatus(DefaultSuspensionStatusHandler).
		WithCustomSuspendMutation(DefaultSuspendMutationHandler).
		WithCustomSuspendDeletionDecision(DefaultDeleteOnSuspendHandler)

	return &Builder{
		base: base,
	}
}

// WithMutation registers a feature-based mutation for the Deployment.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// They are typically used by Features to inject environment variables,
// arguments, or other configuration into the Deployment's containers.
//
// Since mutations are often version-gated, the provided feature.Mutation
// should contain the logic to determine if and how the mutation is applied
// based on the component's current version or configuration.
func (b *Builder) WithMutation(m feature.Mutation[*Mutator]) *Builder {
	b.base.WithMutation(m)
	return b
}

// WithCustomFieldApplicator sets a custom strategy for applying the desired
// state to the existing Deployment in the cluster.
//
// There is a default field applicator (DefaultFieldApplicator) that overwrites
// the entire spec of the current object with the desired state. Using a custom
// applicator is necessary when:
//   - Other controllers (e.g., HPA) manage specific fields like 'replicas'.
//   - Sidecar injectors add containers or volumes that should be preserved.
//   - Defaulting webhooks add fields that would otherwise cause perpetual diffs.
//
// The applicator function receives both the 'current' object from the API
// server and the 'desired' object from the Resource. It is responsible for
// merging the desired changes into the current object.
//
// If a custom applicator is set, it overrides the default baseline application
// logic. Post-application flavors and mutations are still applied afterward.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current *appsv1.Deployment, desired *appsv1.Deployment) error,
) *Builder {
	b.base.WithCustomFieldApplicator(applicator)
	return b
}

// WithFieldApplicationFlavor registers a reusable post-application "flavor" for
// the Deployment.
//
// Flavors are applied in the order they are registered, after the baseline field
// applicator (default or custom) has already run. They are typically used to
// preserve selected live fields from the current object that should not be
// overwritten by the desired state.
//
// If the provided flavor is nil, it is ignored.
func (b *Builder) WithFieldApplicationFlavor(flavor FieldApplicationFlavor) *Builder {
	b.base.WithFieldApplicationFlavor(generic.FieldApplicationFlavor[*appsv1.Deployment](flavor))
	return b
}

// WithCustomConvergeStatus overrides the default logic for determining if the
// Deployment has reached its desired state.
//
// The default behavior uses DefaultConvergingStatusHandler, which considers a
// Deployment ready when its ReadyReplicas count matches the desired replica count.
// Use this method if your Deployment requires more complex health checks, such
// as waiting for specific annotations, status conditions, or external signals.
//
// If you want to augment the default behavior, you can call DefaultConvergingStatusHandler
// within your custom handler.
func (b *Builder) WithCustomConvergeStatus(
	handler func(component.ConvergingOperation, *appsv1.Deployment) (component.ConvergingStatusWithReason, error),
) *Builder {
	b.base.WithCustomConvergeStatus(handler)
	return b
}

// WithCustomGraceStatus overrides how the Deployment reports its health while
// it is still converging (e.g., during a rollout).
//
// The default behavior uses DefaultGraceStatusHandler.
//
// This is used to provide more granular feedback in the component's status
// about the severity of a rollout's progress or failure.
//
// If you want to augment the default behavior, you can call DefaultGraceStatusHandler
// within your custom handler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*appsv1.Deployment) (component.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which reports the
// progress of scaling down to zero replicas. Use this if your custom suspension
// strategy involves other measurable states.
//
// If you want to augment the default behavior, you can call DefaultSuspensionStatusHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*appsv1.Deployment) (component.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the Deployment should be modified when
// the component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which scales the
// Deployment to zero replicas. You might override this if you want to suspend
// the workload by other means, such as changing labels, annotations, or
// updating a 'suspended' field in a custom controller.
//
// If you want to augment the default behavior, you can call DefaultSuspendMutationHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendMutation(
	handler func(*Mutator) error,
) *Builder {
	b.base.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the decision of whether to delete
// the Deployment when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which does not
// delete Deployments during suspension (only scaled down). Return true from
// this handler if you want the Deployment to be completely removed from the
// cluster when suspended.
//
// If you want to augment the default behavior, you can call DefaultDeleteOnSuspendHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*appsv1.Deployment) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithDataExtractor registers a function to harvest information from the
// Deployment after it has been successfully reconciled.
//
// This is useful for capturing auto-generated fields (like names or assigned
// IPs) and making them available to other components or resources via the
// framework's data extraction mechanism.
func (b *Builder) WithDataExtractor(
	extractor func(appsv1.Deployment) error,
) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(d *appsv1.Deployment) error {
			return extractor(*d)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It ensures that:
//   - A base Deployment object was provided.
//   - The Deployment has both a name and a namespace set.
//
// If validation fails, an error is returned and the Resource should not be used.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
