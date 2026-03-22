package job

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	batchv1 "k8s.io/api/batch/v1"
)

// Builder is a configuration helper for creating and customizing a Job Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. This builder ensures that the resulting Resource is
// properly initialized and validated before use in a reconciliation loop.
type Builder struct {
	base *generic.TaskBuilder[*batchv1.Job, *Mutator]
}

// NewBuilder initializes a new Builder with the provided Job object.
//
// The Job object passed here serves as the "desired base state". During
// reconciliation, the Resource will attempt to make the cluster's state match
// this base state, modified by any registered mutations.
//
// The provided Job must have at least a Name and Namespace set, which
// is validated during the Build() call.
func NewBuilder(job *batchv1.Job) *Builder {
	identityFunc := func(j *batchv1.Job) string {
		return fmt.Sprintf("batch/v1/Job/%s/%s", j.Namespace, j.Name)
	}

	base := generic.NewTaskBuilder[*batchv1.Job, *Mutator](
		job,
		identityFunc,
		DefaultFieldApplicator,
		NewMutator,
	)

	base.
		WithCustomConvergeStatus(DefaultConvergingStatusHandler).
		WithCustomSuspendStatus(DefaultSuspensionStatusHandler).
		WithCustomSuspendMutation(DefaultSuspendMutationHandler).
		WithCustomSuspendDeletionDecision(DefaultDeleteOnSuspendHandler)

	return &Builder{
		base: base,
	}
}

// WithMutation registers a feature-based mutation for the Job.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// They are typically used by Features to inject environment variables,
// arguments, or other configuration into the Job's containers.
//
// Since mutations are often version-gated, the provided feature.Mutation
// should contain the logic to determine if and how the mutation is applied
// based on the component's current version or configuration.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithCustomFieldApplicator sets a custom strategy for applying the desired
// state to the existing Job in the cluster.
//
// There is a default field applicator (DefaultFieldApplicator) that overwrites
// the entire spec of the current object with the desired state. Using a custom
// applicator is necessary when external controllers manage specific fields that
// should not be overwritten.
//
// If a custom applicator is set, it overrides the default baseline application
// logic. Post-application flavors and mutations are still applied afterward.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current *batchv1.Job, desired *batchv1.Job) error,
) *Builder {
	b.base.WithCustomFieldApplicator(applicator)
	return b
}

// WithFieldApplicationFlavor registers a reusable post-application "flavor" for
// the Job.
//
// Flavors are applied in the order they are registered, after the baseline field
// applicator (default or custom) has already run. They are typically used to
// preserve selected live fields from the current object that should not be
// overwritten by the desired state.
//
// If the provided flavor is nil, it is ignored.
func (b *Builder) WithFieldApplicationFlavor(flavor FieldApplicationFlavor) *Builder {
	b.base.WithFieldApplicationFlavor(generic.FieldApplicationFlavor[*batchv1.Job](flavor))
	return b
}

// WithCustomConvergeStatus overrides the default logic for determining if the
// Job has completed, is running, or has failed.
//
// The default behavior uses DefaultConvergingStatusHandler, which checks the Job's
// status conditions. Use this method if your Job requires more complex checks,
// such as waiting for specific annotations or external signals.
//
// If you want to augment the default behavior, you can call DefaultConvergingStatusHandler
// within your custom handler.
func (b *Builder) WithCustomConvergeStatus(
	handler func(concepts.ConvergingOperation, *batchv1.Job) (concepts.CompletionStatusWithReason, error),
) *Builder {
	b.base.WithCustomConvergeStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which reports the
// progress based on the Job's Suspend field and active pod count. Use this if
// your custom suspension strategy involves other measurable states.
//
// If you want to augment the default behavior, you can call DefaultSuspensionStatusHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*batchv1.Job) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the Job should be modified when
// the component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which sets the
// Job's Suspend field to true. You might override this if you want to suspend
// the Job by other means.
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
// the Job when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which returns true,
// meaning Jobs are deleted during suspension. Return false from this handler if
// you want the Job to remain in the cluster when suspended.
//
// If you want to augment the default behavior, you can call DefaultDeleteOnSuspendHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*batchv1.Job) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithDataExtractor registers a function to harvest information from the
// Job after it has been successfully reconciled.
//
// This is useful for capturing auto-generated fields (like completion status
// or pod names) and making them available to other components or resources via
// the framework's data extraction mechanism.
func (b *Builder) WithDataExtractor(
	extractor func(batchv1.Job) error,
) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(j *batchv1.Job) error {
			return extractor(*j)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It ensures that:
//   - A base Job object was provided.
//   - The Job has both a name and a namespace set.
//
// If validation fails, an error is returned and the Resource should not be used.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
