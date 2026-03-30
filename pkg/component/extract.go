package component

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// extractResourceData iterates over a list of resources and calls ExtractData for those
// that implement the DataExtractable interface.
//
// For creation resources, this is called per-resource immediately after each apply so that
// extracted data is available to subsequent resources. For read-only resources, this is
// called once after all read-only resources have been fetched.
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
