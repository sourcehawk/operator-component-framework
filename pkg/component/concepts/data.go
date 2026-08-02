package concepts

import (
	"errors"
	"fmt"
)

// ErrDataNotExtracted is returned (wrapped) by Data.Require when the cell has
// not been set during the current reconcile. Callers can match it with
// errors.Is to distinguish "not extracted yet" from other failures.
var ErrDataNotExtracted = errors.New("data not extracted")

// DataCell is the non-generic view of a *Data[T] cell. It lets untyped code
// (builders, the component, introspection) hold heterogeneous cells without
// knowing their value type. Every *Data[T] satisfies it.
type DataCell interface {
	// Name returns the diagnostic name of the cell. Cell identity is the
	// pointer; the name exists for validation messages and introspection.
	Name() string
	// IsSet reports whether the cell currently holds an extracted value.
	IsSet() bool
	// Clear resets the cell's value and presence. It is called by the owning
	// component at the start of each reconcile; calling it from user code is
	// unsupported.
	Clear()
}

// Data is a named, typed, presence-aware cell for intra-component data flow.
// A cell is written by a declared extraction (ExtractInto on a builder) and
// read by later resources' guards and mutations within the same reconcile.
//
// Create cells inside the component assembly function so they stay scoped to
// a single reconcile. As a hardening, the owning component clears every
// declared cell at the start of each reconcile, so accidental reuse of a
// long-lived cell cannot leak state between reconciles. Sharing a cell across
// components is unsupported: validation and reset are per component.
//
// The presence flag separates "not extracted" from "extracted as the zero
// value". There is deliberately no panicking accessor: reconciler code must
// degrade to conditions and requeues, never crash the manager.
type Data[T any] struct {
	name  string
	value T
	set   bool
}

// NewData creates a new, unset data cell with the given diagnostic name.
// Within one component, no two distinct cells may share a name; the component
// builder rejects the collision at Build time.
func NewData[T any](name string) *Data[T] {
	return &Data[T]{name: name}
}

// Name returns the diagnostic name of the cell.
func (d *Data[T]) Name() string { return d.name }

// IsSet reports whether the cell currently holds an extracted value.
func (d *Data[T]) IsSet() bool { return d.set }

// Get returns the cell's value and whether it has been set. When the cell is
// unset, the value is the zero value of T.
func (d *Data[T]) Get() (T, bool) { return d.value, d.set }

// Require returns the cell's value, or the zero value of T and an error
// wrapping ErrDataNotExtracted (naming the cell) when the cell is unset.
// Mutations propagate the error through their normal error path.
func (d *Data[T]) Require() (T, error) {
	if !d.set {
		var zero T
		return zero, fmt.Errorf("data %q: %w", d.name, ErrDataNotExtracted)
	}
	return d.value, nil
}

// Set stores a value in the cell and marks it present. Set is called by
// declared extractions (ExtractInto). The one supported manual use is a test
// seeding a cell before rendering a cluster-free preview; any other manual
// call bypasses topology validation and is unsupported.
func (d *Data[T]) Set(value T) {
	d.value = value
	d.set = true
}

// Clear resets the cell to unset and the zero value of T. Clear is called by
// the owning component at the start of each reconcile.
func (d *Data[T]) Clear() {
	var zero T
	d.value = zero
	d.set = false
}
