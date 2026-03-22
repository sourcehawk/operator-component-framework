// Package features provides sample features for the replicaset primitive.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/replicaset"
)

// PreserveLabelsFlavor demonstrates using a flavor to keep external labels.
func PreserveLabelsFlavor() replicaset.FieldApplicationFlavor {
	return replicaset.PreserveCurrentLabels
}

// PreserveAnnotationsFlavor demonstrates using a flavor to keep external annotations.
func PreserveAnnotationsFlavor() replicaset.FieldApplicationFlavor {
	return replicaset.PreserveCurrentAnnotations
}
