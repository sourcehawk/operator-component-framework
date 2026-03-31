package cronjob

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	batchv1 "k8s.io/api/batch/v1"
)

// DefaultOperationalStatusHandler is the default logic for determining if a CronJob is operational.
//
// It always reports Operational. A CronJob is a passive scheduler: once it exists in the cluster
// it is functioning correctly regardless of whether it has fired yet. The schedule interval may
// be longer than the component's grace period, so treating a never-scheduled CronJob as Pending
// would produce false degradation signals. Failures are reported on the spawned Job resources,
// not on the CronJob itself.
//
// Users who need visibility into whether the CronJob has executed can override this handler via
// Builder.WithCustomConvergeStatus.
func DefaultOperationalStatusHandler(
	_ concepts.ConvergingOperation, _ *batchv1.CronJob,
) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusOperational,
		Reason: "CronJob is a passive scheduler and is considered operational once it exists",
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

// DefaultGraceStatusHandler provides the default health assessment of a CronJob when the
// component's grace period has expired.
//
// It always reports Healthy. A CronJob is a passive scheduler — once it exists, it is
// functioning correctly regardless of whether it has fired yet. The schedule interval may be
// longer than the grace period (e.g. monthly), so waiting for the first execution would
// produce false degradation signals.
//
// This function is used as the default handler by the Resource if no custom handler is
// registered via Builder.WithCustomGraceStatus. It can be reused within custom handlers
// to augment the default behavior.
func DefaultGraceStatusHandler(_ *batchv1.CronJob) (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusHealthy,
		Reason: "CronJob is a passive scheduler and is considered healthy once it exists",
	}, nil
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the CronJob
// when the parent component is suspended.
//
// It always returns false, meaning the CronJob is kept in the cluster with spec.suspend set to true.
func DefaultDeleteOnSuspendHandler(_ *batchv1.CronJob) bool {
	return false
}
