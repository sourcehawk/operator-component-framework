package component

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// extractResourceData iterates over a list of resources and calls ExtractData for those
// that implement the DataExtractable interface.
//
// During reconciliation, this is called per-resource immediately after each resource is
// applied or fetched, so that extracted data is available to subsequent resources' guards
// and mutations.
func extractResourceData(resources []Resource) error {
	for _, resource := range resources {
		if extract, ok := resource.(concepts.DataExtractable); ok {
			if err := extract.ExtractData(); err != nil {
				return fmt.Errorf(
					"failed to extract data from resource %s: %w", resource.Identity(), err,
				)
			}
		}
	}

	return nil
}
