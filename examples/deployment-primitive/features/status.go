package features

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	appsv1 "k8s.io/api/apps/v1"
)

// CustomConvergeStatus demonstrates a custom handler for deployment readiness.
func CustomConvergeStatus() func(concepts.ConvergingOperation, *appsv1.Deployment) (concepts.AliveStatusWithReason, error) {
	return func(op concepts.ConvergingOperation, d *appsv1.Deployment) (concepts.AliveStatusWithReason, error) {
		// Use the default logic but add a custom reason or additional checks.
		status, err := deployment.DefaultConvergingStatusHandler(op, d)
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
func CustomGraceStatus() func(*appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
	return func(d *appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
		// Example: If it's a critical component, we might want to report Down
		// even if some replicas are up but not enough.
		if d.Status.ReadyReplicas < 2 {
			return concepts.GraceStatusWithReason{
				Status: concepts.GraceStatusDown,
				Reason: "At least 2 replicas are required for minimal service",
			}, nil
		}

		return deployment.DefaultGraceStatusHandler(d)
	}
}

// CustomSuspendMutation demonstrates a custom mutation when suspended.
func CustomSuspendMutation() func(*deployment.Mutator) error {
	return func(m *deployment.Mutator) error {
		// Default is scaling to 0.
		if err := deployment.DefaultSuspendMutationHandler(m); err != nil {
			return err
		}

		// Additionally, add an annotation indicating when it was suspended.
		m.EditObjectMetadata(func(meta *editors.ObjectMetaEditor) error {
			meta.EnsureAnnotation("example.io/suspended-at", "2026-03-20")
			return nil
		})

		return nil
	}
}
