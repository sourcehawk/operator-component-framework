// Package features provides sample features for the deployment primitive.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
)

// PreserveLabelsFlavor demonstrates using a flavor to keep external labels.
func PreserveLabelsFlavor() deployment.FieldApplicationFlavor {
	return deployment.PreserveCurrentLabels
}

// PreserveAnnotationsFlavor demonstrates using a flavor to keep external annotations.
func PreserveAnnotationsFlavor() deployment.FieldApplicationFlavor {
	return deployment.PreserveCurrentAnnotations
}
