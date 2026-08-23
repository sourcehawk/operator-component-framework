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
// declared data extractions. This builder ensures that the resulting Resource is
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

// WithMutation registers one or more feature-based mutations for the CronJob.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
func (b *Builder) WithMutation(ms ...Mutation) *Builder {
	for _, m := range ms {
		b.base.WithMutation(feature.Mutation[*Mutator](m))
	}
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
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(
	guard func(batchv1.CronJob) (concepts.GuardStatusWithReason, error),
) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataGuard declares that the CronJob reads the given data cells and
// must not be applied until every one of them is set. The framework generates
// the guard and its reason (waiting for data "<name>"), and component Build
// validates that a producer for each cell is registered earlier. Data guards
// are evaluated before any custom guard registered with WithGuard.
func (b *Builder) WithDataGuard(cells ...concepts.DataCell) *Builder {
	b.base.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares that the CronJob reads the given data cells
// without gating on them. Component Build still validates that a producer is
// registered earlier, and the dependency stays visible to introspection.
// Consumers in this mode use Get and skip quietly when a cell is absent.
func (b *Builder) WithOptionalData(cells ...concepts.DataCell) *Builder {
	b.base.WithOptionalData(cells...)
	return b
}

// WithMetricsIdentifier sets the CronJob's identifier for
// resource-level metrics, used as the value of the `resource` label on
// ocf_resource_apply_total and ocf_resource_apply_errors_total.
//
// It is a Prometheus label value, not a Kubernetes name: it must be
// low-cardinality and stable across reconciles, never derived from a per-owner
// value such as the owning custom resource's name. When unset, the resource is
// labelled `cronjob`. Build rejects a blank identifier.
func (b *Builder) WithMetricsIdentifier(identifier string) *Builder {
	b.base.WithMetricsIdentifier(identifier)
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

// ExtractInto declares that this CronJob produces the value of cell. fn
// computes the value from a copy of the reconciled CronJob; the framework
// stores it in the cell and marks it present, immediately after the CronJob
// is applied or fetched. Extracting several values means several ExtractInto
// calls, one per cell. This is a package-level function because Go methods
// cannot introduce the extra type parameter V.
func ExtractInto[V any](b *Builder, cell *concepts.Data[V], fn func(batchv1.CronJob) (V, error)) {
	generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
