// Package features provides sample features for the job primitive.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/job"
)

// PreserveLabelsFlavor demonstrates using a flavor to keep external labels.
func PreserveLabelsFlavor() job.FieldApplicationFlavor {
	return job.PreserveCurrentLabels
}

// PreserveAnnotationsFlavor demonstrates using a flavor to keep external annotations.
func PreserveAnnotationsFlavor() job.FieldApplicationFlavor {
	return job.PreserveCurrentAnnotations
}
