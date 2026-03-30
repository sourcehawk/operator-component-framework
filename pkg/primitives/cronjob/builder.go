package cronjob

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	batchv1 "k8s.io/api/batch/v1"
)

// Builder is a configuration helper for creating and customizing a CronJob Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. This builder ensures that the resulting Resource is
// properly initialized and validated before use in a reconciliation loop.
type Builder struct {
	base *generic.IntegrationBuilder[*batchv1.CronJob, *Mutator]
}

// NewBuilder initializes a new Builder with the provided CronJob object.
//
// The CronJob object passed here serves as the "desired base state". During
// reconciliation, the Resource will attempt to make the cluster's state match
// this base state, modified by any registered mutations.
//
// The provided CronJob must have at least a Name and Namespace set, which
// is validated during the Build() call.
func NewBuilder(cj *batchv1.CronJob) *Builder {
	identityFunc := func(c *batchv1.CronJob) string {
		return fmt.Sprintf("batch/v1/CronJob/%s/%s", c.Namespace, c.Name)
	}

	base := generic.NewIntegrationBuilder[*batchv1.CronJob, *Mutator](
		cj,
		identityFunc,
		NewMutator,
	)

	base.
		WithCustomOperationalStatus(DefaultOperationalStatusHandler).
		WithCustomGraceStatus(DefaultGraceStatusHandler).
		WithCustomSuspendStatus(DefaultSuspensionStatusHandler).
		WithCustomSuspendMutation(DefaultSuspendMutationHandler).
		WithCustomSuspendDeletionDecision(DefaultDeleteOnSuspendHandler)

	return &Builder{
		base: base,
	}
}

// WithMutation registers a feature-based mutation for the CronJob.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithCustomOperationalStatus overrides the default logic for determining if the
// CronJob is operational.
func (b *Builder) WithCustomOperationalStatus(
	handler func(concepts.ConvergingOperation, *batchv1.CronJob) (concepts.OperationalStatusWithReason, error),
) *Builder {
	b.base.WithCustomOperationalStatus(handler)
	return b
}

// WithCustomGraceStatus overrides the default logic for assessing the health of
// the CronJob when the component's grace period has expired.
//
// The default behavior uses DefaultGraceStatusHandler, which always reports
// Healthy. A CronJob is a passive scheduler — once it exists and is not
// suspended, it is functioning correctly regardless of whether it has fired yet.
//
// If you want to augment the default behavior, you can call DefaultGraceStatusHandler
// within your custom handler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*batchv1.CronJob) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*batchv1.CronJob) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the CronJob should be modified when
// the component is suspended.
func (b *Builder) WithCustomSuspendMutation(
	handler func(*Mutator) error,
) *Builder {
	b.base.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the decision of whether to delete
// the CronJob when the component is suspended.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*batchv1.CronJob) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithGuard registers a guard precondition that is evaluated before the CronJob
// is applied during reconciliation. If the guard returns Blocked, the CronJob and
// all resources registered after it are skipped until the guard clears.
func (b *Builder) WithGuard(
	guard func(batchv1.CronJob) (concepts.GuardStatusWithReason, error),
) *Builder {
	if guard == nil {
		b.base.WithGuard(nil)
		return b
	}
	b.base.WithGuard(func(c *batchv1.CronJob) (concepts.GuardStatusWithReason, error) {
		return guard(*c)
	})
	return b
}

// WithDataExtractor registers a function to harvest information from the
// CronJob after it has been successfully reconciled.
func (b *Builder) WithDataExtractor(
	extractor func(batchv1.CronJob) error,
) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(c *batchv1.CronJob) error {
			return extractor(*c)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It ensures that:
//   - A base CronJob object was provided.
//   - The CronJob has both a name and a namespace set.
//
// If validation fails, an error is returned and the Resource should not be used.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
