package cronjob

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	batchv1 "k8s.io/api/batch/v1"
)

// DefaultOperationalStatusHandler is the default logic for determining if a CronJob is operational.
//
// It considers a CronJob operational when it has scheduled at least once
// (Status.LastScheduleTime is not nil). If it has never been scheduled,
// the status is Pending.
//
// Failures are reported on the spawned Job resources, not on the CronJob itself.
func DefaultOperationalStatusHandler(
	_ concepts.ConvergingOperation, cj *batchv1.CronJob,
) (concepts.OperationalStatusWithReason, error) {
	if cj.Status.LastScheduleTime == nil {
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusPending,
			Reason: "CronJob has never been scheduled",
		}, nil
	}

	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusOperational,
		Reason: fmt.Sprintf("CronJob last scheduled at %s", cj.Status.LastScheduleTime.Format("2006-01-02T15:04:05Z")),
	}, nil
}

// DefaultSuspendMutationHandler provides the default mutation applied to a CronJob when the component is suspended.
//
// It sets spec.suspend to true, which prevents the CronJob from creating new Job objects.
func DefaultSuspendMutationHandler(mutator *Mutator) error {
	mutator.EditCronJobSpec(func(e *editors.CronJobSpecEditor) error {
		suspend := true
		e.Raw().Suspend = &suspend
		return nil
	})
	return nil
}

// DefaultSuspensionStatusHandler monitors the progress of the suspension process.
//
// It reports Suspended when spec.suspend is true and no active jobs are running.
// It reports Suspending when spec.suspend is true but active jobs are still running.
func DefaultSuspensionStatusHandler(cj *batchv1.CronJob) (concepts.SuspensionStatusWithReason, error) {
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		if len(cj.Status.Active) > 0 {
			return concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspending,
				Reason: fmt.Sprintf("CronJob suspended but %d active jobs still running", len(cj.Status.Active)),
			}, nil
		}

		return concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspended,
			Reason: "CronJob suspended",
		}, nil
	}

	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspending,
		Reason: "Waiting for suspend flag to be applied",
	}, nil
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the CronJob
// when the parent component is suspended.
//
// It always returns false, meaning the CronJob is kept in the cluster with spec.suspend set to true.
func DefaultDeleteOnSuspendHandler(_ *batchv1.CronJob) bool {
	return false
}
