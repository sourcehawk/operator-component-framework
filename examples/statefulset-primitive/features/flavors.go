// Package features provides sample features for the statefulset primitive.
package features

import (
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"
	appsv1 "k8s.io/api/apps/v1"
)

// PreserveLabelsFlavor demonstrates using a flavor to keep external labels.
func PreserveLabelsFlavor() statefulset.FieldApplicationFlavor {
	return statefulset.PreserveCurrentLabels
}

// PreserveAnnotationsFlavor demonstrates using a flavor to keep external annotations.
func PreserveAnnotationsFlavor() statefulset.FieldApplicationFlavor {
	return statefulset.PreserveCurrentAnnotations
}

// CustomConvergeStatus demonstrates a custom handler for statefulset readiness.
func CustomConvergeStatus() func(concepts.ConvergingOperation, *appsv1.StatefulSet) (concepts.AliveStatusWithReason, error) {
	return func(op concepts.ConvergingOperation, s *appsv1.StatefulSet) (concepts.AliveStatusWithReason, error) {
		status, err := statefulset.DefaultConvergingStatusHandler(op, s)
		if err != nil {
			return status, err
		}

		if status.Status == concepts.AliveConvergingStatusHealthy {
			status.Reason = "Application is fully operational and healthy"
		} else {
			status.Reason = fmt.Sprintf("Application is warming up: %s", status.Reason)
		}

		return status, nil
	}
}

// CustomGraceStatus demonstrates a custom handler for health when not ready.
func CustomGraceStatus() func(*appsv1.StatefulSet) (concepts.GraceStatusWithReason, error) {
	return func(s *appsv1.StatefulSet) (concepts.GraceStatusWithReason, error) {
		if s.Status.ReadyReplicas < 2 {
			return concepts.GraceStatusWithReason{
				Status: concepts.GraceStatusDown,
				Reason: "At least 2 replicas are required for minimal service",
			}, nil
		}

		return statefulset.DefaultGraceStatusHandler(s)
	}
}

// CustomSuspendMutation demonstrates a custom mutation when suspended.
func CustomSuspendMutation() func(*statefulset.Mutator) error {
	return func(m *statefulset.Mutator) error {
		if err := statefulset.DefaultSuspendMutationHandler(m); err != nil {
			return err
		}

		m.EditObjectMetadata(func(meta *editors.ObjectMetaEditor) error {
			raw := meta.Raw()
			if _, exists := raw.Annotations["example.io/suspended-at"]; !exists {
				meta.EnsureAnnotation("example.io/suspended-at", time.Now().UTC().Format(time.RFC3339))
			}
			return nil
		})

		return nil
	}
}
