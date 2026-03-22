package job

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	batchv1 "k8s.io/api/batch/v1"
)

// DefaultConvergingStatusHandler is the default logic for determining if a Job has completed or is still running.
//
// It checks the Job's status conditions for Complete or Failed, and reports the
// appropriate CompletionStatus based on the current state.
//
// This function is used as the default handler by the Resource if no custom handler
// is registered via Builder.WithCustomConvergeStatus. It can be reused within
// custom handlers to augment the default behavior.
func DefaultConvergingStatusHandler(
	op concepts.ConvergingOperation, job *batchv1.Job,
) (concepts.CompletionStatusWithReason, error) {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == "True" {
			return concepts.CompletionStatusWithReason{
				Status: concepts.CompletionStatusCompleted,
				Reason: "Job completed successfully",
			}, nil
		}
		if cond.Type == batchv1.JobFailed && cond.Status == "True" {
			reason := "Job failed"
			if cond.Message != "" {
				reason = fmt.Sprintf("Job failed: %s", cond.Message)
			}
			return concepts.CompletionStatusWithReason{
				Status: concepts.CompletionStatusFailing,
				Reason: reason,
			}, nil
		}
	}

	if job.Status.Active > 0 {
		return concepts.CompletionStatusWithReason{
			Status: concepts.CompletionStatusRunning,
			Reason: fmt.Sprintf("Job has %d active pod(s)", job.Status.Active),
		}, nil
	}

	switch op {
	case concepts.ConvergingOperationCreated:
		return concepts.CompletionStatusWithReason{
			Status: concepts.CompletionStatusPending,
			Reason: "Job was just created, waiting for pods to be scheduled",
		}, nil
	default:
		return concepts.CompletionStatusWithReason{
			Status: concepts.CompletionStatusPending,
			Reason: "Job is waiting for pods to be scheduled",
		}, nil
	}
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the Job
// when the parent component is suspended.
//
// It always returns true, meaning the Job is deleted from the cluster during suspension.
// Jobs cannot be meaningfully scaled to zero like Deployments, so deletion is the
// standard approach for suspending a task resource.
//
// This function is used as the default handler by the Resource if no custom handler is
// registered via Builder.WithCustomSuspendDeletionDecision. It can be reused within
// custom handlers.
func DefaultDeleteOnSuspendHandler(_ *batchv1.Job) bool {
	return true
}

// DefaultSuspendMutationHandler provides the default mutation applied to a Job when
// the component is suspended.
//
// It sets the Job's Suspend field to true, which prevents the Job controller from
// creating new pods while allowing existing pods to complete.
//
// This function is used as the default handler by the Resource if no custom handler is
// registered via Builder.WithCustomSuspendMutation. It can be reused within custom handlers.
func DefaultSuspendMutationHandler(mutator *Mutator) error {
	mutator.EditJobSpec(func(e *editors.JobSpecEditor) error {
		e.Raw().Suspend = boolPtr(true)
		return nil
	})
	return nil
}

// DefaultSuspensionStatusHandler monitors the progress of the suspension process.
//
// It reports whether the Job has been successfully suspended by checking if the
// Job's Suspend field is true and there are no active pods.
//
// This function is used as the default handler by the Resource if no custom handler is
// registered via Builder.WithCustomSuspendStatus. It can be reused within custom handlers.
func DefaultSuspensionStatusHandler(job *batchv1.Job) (concepts.SuspensionStatusWithReason, error) {
	isSuspended := job.Spec.Suspend != nil && *job.Spec.Suspend

	if isSuspended && job.Status.Active == 0 {
		return concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspended,
			Reason: "Job is suspended",
		}, nil
	}

	if isSuspended {
		return concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspending,
			Reason: fmt.Sprintf("Job is suspending, %d pod(s) still active", job.Status.Active),
		}, nil
	}

	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspending,
		Reason: "Waiting for Job suspend field to be applied",
	}, nil
}

func boolPtr(b bool) *bool {
	return &b
}
