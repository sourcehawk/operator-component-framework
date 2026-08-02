package generic

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// BaseBuilder provides shared behavior for all generic internal resource builders.
type BaseBuilder[T client.Object, M FeatureMutator] struct {
	BaseRes      *BaseResource[T, M]
	clusterScope bool
}

// InitBase initializes the base resource configuration.
//
// Safe defaults are configured for suspension handlers so that custom resource
// wrappers built on the generic layer do not need to set them explicitly:
//   - SuspendMutationHandler: no-op (does nothing on suspend)
//   - SuspendStatusHandler: reports Suspended immediately
//
// DeleteOnSuspend defaults to false via a nil-check in BaseResource.DeleteOnSuspend.
func (b *BaseBuilder[T, M]) InitBase(
	obj T,
	identityFunc func(T) string,
	newMutator func(T) M,
) {
	b.BaseRes = &BaseResource[T, M]{
		DesiredObject: obj,
		IdentityFunc:  identityFunc,
		NewMutator:    newMutator,
		SuspendMutationHandler: func(_ M) error {
			return nil
		},
		SuspendStatusHandler: func(_ T) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: "default suspension status",
			}, nil
		},
	}
}

// WithMutation registers one or more typed feature mutations for the resource.
// Mutations are appended in the order provided; calling it with no arguments is
// a no-op.
func (b *BaseBuilder[T, M]) WithMutation(ms ...Mutation[M]) {
	b.BaseRes.Mutations = append(b.BaseRes.Mutations, ms...)
}

// WithGuard registers a guard precondition for the resource.
// The guard is evaluated before each apply during reconciliation. If it returns
// Blocked, the resource and all resources after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *BaseBuilder[T, M]) WithGuard(handler func(T) (concepts.GuardStatusWithReason, error)) {
	b.BaseRes.GuardHandler = handler
}

// WithDataGuard declares that the resource reads the given cells and must not
// be applied until every one of them is set. The framework generates the guard
// and its reason (for example: waiting for data "db-host"), so the reason can
// never drift from the actual dependency. A blocked data guard surfaces as the
// same Blocked condition reason custom guards produce.
//
// Data guards are evaluated before any custom guard registered with WithGuard;
// both may be combined. Component Build validates that a producer for each
// cell is registered strictly earlier in the component.
func (b *BaseBuilder[T, M]) WithDataGuard(cells ...concepts.DataCell) {
	for _, cell := range cells {
		b.BaseRes.DataConsumptions = append(
			b.BaseRes.DataConsumptions,
			concepts.DataConsumption{Cell: cell, Optional: false},
		)
	}
}

// WithOptionalData declares that the resource reads the given cells without
// gating on them. The declaration exists so component Build still verifies a
// producer is registered earlier (an optional read with no producer is
// permanently absent, which is dead code and almost certainly a bug) and so
// the dependency stays visible to introspection. Consumers in this mode use
// Get and skip quietly when the cell is absent.
func (b *BaseBuilder[T, M]) WithOptionalData(cells ...concepts.DataCell) {
	for _, cell := range cells {
		b.BaseRes.DataConsumptions = append(
			b.BaseRes.DataConsumptions,
			concepts.DataConsumption{Cell: cell, Optional: true},
		)
	}
}

// WithCustomSuspendStatus overrides the resource suspension status handler.
func (b *BaseBuilder[T, M]) WithCustomSuspendStatus(
	handler func(T) (concepts.SuspensionStatusWithReason, error),
) {
	b.BaseRes.SuspendStatusHandler = handler
}

// WithCustomSuspendMutation overrides the resource suspension mutation handler.
func (b *BaseBuilder[T, M]) WithCustomSuspendMutation(handler func(M) error) {
	b.BaseRes.SuspendMutationHandler = handler
}

// WithCustomSuspendDeletionDecision overrides the resource delete-on-suspend decision handler.
func (b *BaseBuilder[T, M]) WithCustomSuspendDeletionDecision(handler func(T) bool) {
	b.BaseRes.DeleteOnSuspendHandler = handler
}

// MarkClusterScoped marks the resource as cluster-scoped. ValidateBase will
// reject a non-empty namespace instead of requiring one.
func (b *BaseBuilder[T, M]) MarkClusterScoped() {
	b.clusterScope = true
}

// ValidateBase validates the base resource configuration.
func (b *BaseBuilder[T, M]) ValidateBase() error {
	if isNil(b.BaseRes.DesiredObject) {
		return errors.New("object cannot be nil")
	}

	if b.BaseRes.DesiredObject.GetName() == "" {
		return errors.New("object name cannot be empty")
	}

	if b.clusterScope {
		if b.BaseRes.DesiredObject.GetNamespace() != "" {
			return errors.New("cluster-scoped object must not have a namespace")
		}
	} else {
		if b.BaseRes.DesiredObject.GetNamespace() == "" {
			return errors.New("object namespace cannot be empty")
		}
	}

	if b.BaseRes.IdentityFunc == nil {
		return errors.New("identity function cannot be nil")
	}

	if b.BaseRes.NewMutator == nil {
		return errors.New("mutator factory cannot be nil")
	}

	// Declared data extractions must reference a real cell and a real
	// extraction function. A typed-nil cell or nil fn passed to ExtractInto is
	// recorded and rejected here so the failure surfaces at build time with a
	// clear message instead of panicking mid-reconcile.
	for _, extraction := range b.BaseRes.DataExtractions {
		if isNil(extraction.Cell) {
			return errors.New("declared data extraction requires a non-nil cell")
		}
		if extraction.Extract == nil {
			return fmt.Errorf(
				"declared data extraction into %q requires a non-nil extraction function",
				extraction.Cell.Name(),
			)
		}
	}

	// Declared data reads must reference a real cell for the same reason.
	for _, consumption := range b.BaseRes.DataConsumptions {
		if isNil(consumption.Cell) {
			return errors.New("declared data read (WithDataGuard or WithOptionalData) requires a non-nil cell")
		}
	}

	// Mutation names must be unique within a resource. A name is the identifier
	// that gating and error reporting refer to, so two mutations sharing one is
	// ambiguous: it silently masks a mis-targeted or dead mutation behind its
	// namesake. This is a name-only check and evaluates no feature gates.
	seen := make(map[string]struct{}, len(b.BaseRes.Mutations))
	for _, m := range b.BaseRes.Mutations {
		if _, dup := seen[m.Name]; dup {
			return fmt.Errorf("duplicate mutation name %q: mutation names must be unique within a resource", m.Name)
		}
		seen[m.Name] = struct{}{}
	}

	return nil
}
