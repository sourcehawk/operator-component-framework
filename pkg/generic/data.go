package generic

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DataExtraction records one declared write of a data cell by a resource: the
// destination cell plus the function that computes and stores its value. It is
// recorded by ExtractInto; constructing it manually is unsupported.
type DataExtraction[T client.Object] struct {
	// Cell is the destination cell, held through the non-generic DataCell view.
	Cell concepts.DataCell
	// Extract computes the value from the reconciled object and stores it in
	// the cell. A nil Extract is rejected at Build time.
	Extract func(T) error
}

// ExtractInto declares that the resource built by b produces the value of
// cell. fn computes the value from the reconciled object; the framework stores
// it in the cell and marks it present. The extraction runs immediately after
// the resource is applied or fetched, before subsequent resources reconcile.
//
// This is a package-level function rather than a builder method because Go
// methods cannot introduce the extra type parameter V.
//
// Extracting several values from one object means several ExtractInto calls,
// one per cell. Multiple resources may produce the same cell; the last
// registered producer's extraction wins at runtime.
//
// A nil cell or nil fn is rejected when the builder's Build method runs.
func ExtractInto[T client.Object, M FeatureMutator, V any](
	b *BaseBuilder[T, M], cell *concepts.Data[V], fn func(T) (V, error),
) {
	extraction := DataExtraction[T]{}
	if cell != nil {
		extraction.Cell = cell
	}
	if fn != nil {
		extraction.Extract = func(obj T) error {
			v, err := fn(obj)
			if err != nil {
				return err
			}
			cell.Set(v)
			return nil
		}
	}
	b.BaseRes.DataExtractions = append(b.BaseRes.DataExtractions, extraction)
}

// WrapExtraction converts a value-receiver extraction callback into a
// pointer-receiver callback suitable for the generic layer's ExtractInto.
// If the input function is nil, nil is returned.
func WrapExtraction[E any, V any](fn func(E) (V, error)) func(*E) (V, error) {
	if fn == nil {
		return nil
	}
	return func(ptr *E) (V, error) {
		return fn(*ptr)
	}
}
