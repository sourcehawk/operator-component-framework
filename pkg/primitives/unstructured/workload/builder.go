package workload

import (
	"errors"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Builder is a configuration helper for creating and customizing an unstructured
// workload Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. The converging status handler is required; all other
// handlers default to safe no-ops when omitted.
type Builder struct {
	base         *generic.WorkloadBuilder[*uns.Unstructured, *unstruct.Mutator]
	clusterScope bool

	hasConvergeStatus bool
}

// NewBuilder initializes a new Builder with the provided unstructured object.
//
// The object serves as the desired base state. The converging status handler
// must be set via WithCustomConvergeStatus before Build(). All other handlers
// are optional.
func NewBuilder(obj *uns.Unstructured) *Builder {
	placeholder := func(_ *uns.Unstructured) string { return "" }

	base := generic.NewWorkloadBuilder[*uns.Unstructured, *unstruct.Mutator](
		obj,
		placeholder,
		unstruct.NewMutator,
	)

	base.WithCustomGraceStatus(unstruct.DefaultGraceStatusHandler)
	base.WithCustomSuspendMutation(unstruct.DefaultSuspendMutationHandler)
	base.WithCustomSuspendStatus(unstruct.DefaultSuspensionStatusHandler)

	return &Builder{base: base}
}

// MarkClusterScoped marks the resource as cluster-scoped.
func (b *Builder) MarkClusterScoped() *Builder {
	b.clusterScope = true
	b.base.MarkClusterScoped()
	return b
}

// WithMutation registers a mutation for the unstructured object.
func (b *Builder) WithMutation(m unstruct.Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*unstruct.Mutator](m))
	return b
}

// WithCustomConvergeStatus sets the handler that evaluates whether the resource
// has reached its desired state. This handler is required.
func (b *Builder) WithCustomConvergeStatus(
	handler func(concepts.ConvergingOperation, *uns.Unstructured) (concepts.AliveStatusWithReason, error),
) *Builder {
	b.hasConvergeStatus = true
	b.base.WithCustomConvergeStatus(handler)
	return b
}

// WithCustomGraceStatus overrides the default grace status handler that assesses
// health during rollouts. The default reports Healthy.
func (b *Builder) WithCustomGraceStatus(
	handler func(*uns.Unstructured) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides the default suspension status handler.
// The default reports Suspended immediately.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*uns.Unstructured) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation overrides the default suspension mutation handler.
// The default is a no-op.
func (b *Builder) WithCustomSuspendMutation(
	handler func(*unstruct.Mutator) error,
) *Builder {
	b.base.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the default delete-on-suspend
// decision. The default returns false (keep the resource).
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*uns.Unstructured) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithDataExtractor registers a function to read values from the object after
// it has been successfully reconciled.
func (b *Builder) WithDataExtractor(extractor func(uns.Unstructured) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(obj *uns.Unstructured) error {
			return extractor(*obj)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if the converging status handler has not been set.
func (b *Builder) Build() (*Resource, error) {
	if err := b.validateHandlers(); err != nil {
		return nil, err
	}
	b.base.BaseRes.IdentityFunc = unstruct.MakeIdentityFunc(b.clusterScope)
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}

func (b *Builder) validateHandlers() error {
	if !b.hasConvergeStatus {
		return errors.New("converging status handler is required")
	}
	return nil
}
