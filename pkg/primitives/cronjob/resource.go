// Package cronjob provides a builder and resource for managing Kubernetes CronJobs.
package cronjob

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes CronJob within a controller's
// reconciliation loop.
//
// It implements several component interfaces to integrate with the operator-component-framework:
//   - component.Resource: for basic identity and mutation behavior.
//   - component.Operational: for operational status tracking.
//   - component.Suspendable: for controlled suspension via spec.suspend.
//   - component.DataExtractable: for exporting information after successful reconciliation.
type Resource struct {
	base *generic.IntegrationResource[*batchv1.CronJob, *Mutator]
}

// Identity returns a unique identifier for the CronJob in the format
// "batch/v1/CronJob/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a copy of the underlying Kubernetes CronJob object.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes CronJob into the desired state.
//
// The mutation process follows a specific order:
//  1. Feature Mutations: All registered feature-based mutations are applied.
//  2. Suspension: If the resource is in a suspending state, the suspension
//     logic (setting spec.suspend = true) is applied.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus reports the CronJob's operational status.
//
// By default, it uses DefaultOperationalStatusHandler, which reports Pending
// when the CronJob has never been scheduled and Operational when it has
// scheduled at least once.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// DeleteOnSuspend determines whether the CronJob should be deleted from the
// cluster when the parent component is suspended.
//
// By default, it returns false — the CronJob is kept with spec.suspend set to true.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the suspension of the CronJob.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behavior sets spec.suspend to true.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which reports Suspended
// when spec.suspend is true and no active jobs are running.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes registered data extraction functions to harvest information
// from the reconciled CronJob.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
