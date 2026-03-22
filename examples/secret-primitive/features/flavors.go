package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/secret"
)

// PreserveExternalEntriesFlavor demonstrates using a flavor to keep Secret
// entries that were added by other controllers or tooling (e.g., admission
// webhooks, manual operations) without the operator overwriting them.
func PreserveExternalEntriesFlavor() secret.FieldApplicationFlavor {
	return secret.PreserveExternalEntries
}
