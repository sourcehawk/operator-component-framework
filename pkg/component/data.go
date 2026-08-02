package component

import (
	"fmt"
	"reflect"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// isNilCell reports whether the cell is a nil interface or an interface holding
// a typed-nil value such as (*concepts.Data[string])(nil). Both forms panic on
// the first method call, so validation rejects them with a build error rather
// than letting the panic escape. It mirrors isNilResource in builder.go.
func isNilCell(cell concepts.DataCell) bool {
	if cell == nil {
		return true
	}
	v := reflect.ValueOf(cell)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

// validateDataTopology walks resources in registration order and validates the
// component's declared data flow:
//
//  1. Every cell a resource reads (guarded or optional) has at least one
//     producer registered strictly earlier.
//  2. No two distinct cells within the component share a name. Pointer
//     identity is what the checks run on; the name collision check exists so
//     diagnostics and introspection stay unambiguous.
//
// It returns the declared cells in first-producer registration order (the set
// the component clears at the start of each reconcile) and all violations
// found. Only reconcile resources participate: delete and orphan resources
// never run extraction, so data declared on them is not considered.
func validateDataTopology(componentName string, entries []reconcileEntry) ([]concepts.DataCell, []error) {
	var errs []error
	produced := make(map[concepts.DataCell]struct{})
	names := make(map[string]concepts.DataCell)
	var cells []concepts.DataCell

	checkName := func(identity string, cell concepts.DataCell) {
		existing, ok := names[cell.Name()]
		if !ok {
			names[cell.Name()] = cell
			return
		}
		if existing != cell {
			errs = append(errs, fmt.Errorf(
				"resource %q in component %q declares data %q, but a distinct cell already uses that name; data names must be unique within a component",
				identity, componentName, cell.Name(),
			))
		}
	}

	for _, entry := range entries {
		identity := entry.Resource.Identity()

		// Reads are checked before this resource's own writes so that a
		// producer can never satisfy its own read: the producer must be
		// registered strictly earlier.
		if consumer, ok := entry.Resource.(concepts.DataConsumer); ok {
			for _, consumption := range consumer.ConsumedData() {
				if isNilCell(consumption.Cell) {
					errs = append(errs, fmt.Errorf(
						"resource %q in component %q declares a nil data cell read", identity, componentName,
					))
					continue
				}
				checkName(identity, consumption.Cell)
				if _, ok := produced[consumption.Cell]; !ok {
					errs = append(errs, fmt.Errorf(
						"resource %q reads data %q but no earlier resource produces it",
						identity, consumption.Cell.Name(),
					))
				}
			}
		}

		if producer, ok := entry.Resource.(concepts.DataProducer); ok {
			for _, cell := range producer.ProducedData() {
				if isNilCell(cell) {
					errs = append(errs, fmt.Errorf(
						"resource %q in component %q declares a nil data cell write", identity, componentName,
					))
					continue
				}
				checkName(identity, cell)
				if _, dup := produced[cell]; !dup {
					produced[cell] = struct{}{}
					cells = append(cells, cell)
				}
			}
		}
	}

	return cells, errs
}

// DataTopology returns one edge per declared data cell, in first-producer
// registration order. Within an edge, producers and readers are listed in
// registration order. It satisfies concepts.DataInspector, giving tests and
// tooling the same read-only view of data flow that MutationInspector gives
// for mutations. Nothing in the reconcile path calls it.
func (c *Component) DataTopology() []concepts.DataEdge {
	edges := make(map[concepts.DataCell]*concepts.DataEdge)
	var order []concepts.DataCell

	for _, entry := range c.reconcileResources {
		identity := entry.Resource.Identity()

		if producer, ok := entry.Resource.(concepts.DataProducer); ok {
			for _, cell := range producer.ProducedData() {
				edge, ok := edges[cell]
				if !ok {
					edge = &concepts.DataEdge{Data: cell.Name()}
					edges[cell] = edge
					order = append(order, cell)
				}
				edge.Producers = append(edge.Producers, identity)
			}
		}

		if consumer, ok := entry.Resource.(concepts.DataConsumer); ok {
			for _, consumption := range consumer.ConsumedData() {
				edge, ok := edges[consumption.Cell]
				if !ok {
					// Build validation guarantees every read has an earlier
					// producer, so this only guards a hand-built Component.
					continue
				}
				if consumption.Optional {
					edge.Optional = append(edge.Optional, identity)
				} else {
					edge.Guarded = append(edge.Guarded, identity)
				}
			}
		}
	}

	out := make([]concepts.DataEdge, 0, len(order))
	for _, cell := range order {
		out = append(out, *edges[cell])
	}
	return out
}
