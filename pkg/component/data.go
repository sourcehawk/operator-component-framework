package component

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

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
				if consumption.Cell == nil {
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
				if cell == nil {
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
