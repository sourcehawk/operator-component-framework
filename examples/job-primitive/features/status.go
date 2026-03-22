package features

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/job"
	batchv1 "k8s.io/api/batch/v1"
)

// CustomConvergeStatus demonstrates a custom handler for job completion status.
func CustomConvergeStatus() func(concepts.ConvergingOperation, *batchv1.Job) (concepts.CompletionStatusWithReason, error) {
	return func(op concepts.ConvergingOperation, j *batchv1.Job) (concepts.CompletionStatusWithReason, error) {
		// Use the default logic but add a custom reason or additional checks.
		status, err := job.DefaultConvergingStatusHandler(op, j)
		if err != nil {
			return status, err
		}

		switch status.Status {
		case concepts.CompletionStatusCompleted:
			status.Reason = "Migration completed successfully"
		case concepts.CompletionStatusRunning:
			status.Reason = fmt.Sprintf("Migration in progress: %s", status.Reason)
		}

		return status, nil
	}
}
