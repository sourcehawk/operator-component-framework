package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
)

// PreserveExternalEntriesFlavor demonstrates using a flavor to keep ConfigMap
// entries that were added by other controllers or tooling (e.g., admission
// webhooks, manual operations) without the operator overwriting them.
func PreserveExternalEntriesFlavor() configmap.FieldApplicationFlavor {
	return configmap.PreserveExternalEntries
}
