package component

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// extractResourceData iterates over a list of resources and calls ExtractData for those
// that implement the DataExtractable interface.
//
// This helper is called during reconciliation after both registered creation and
// read-only resources have been successfully processed and updated with their cluster state.
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
