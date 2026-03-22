// Package features provides sample features for the daemonset primitive.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/daemonset"
)

// PreserveLabelsFlavor demonstrates using a flavor to keep external labels.
func PreserveLabelsFlavor() daemonset.FieldApplicationFlavor {
	return daemonset.PreserveCurrentLabels
}

// PreserveAnnotationsFlavor demonstrates using a flavor to keep external annotations.
func PreserveAnnotationsFlavor() daemonset.FieldApplicationFlavor {
	return daemonset.PreserveCurrentAnnotations
}
