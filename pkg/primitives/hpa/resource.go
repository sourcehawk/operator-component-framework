package hpa

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultFieldApplicator replaces current with a deep copy of desired while
// preserving server-managed metadata (ResourceVersion, UID, Generation, etc.)
// and shared-controller fields (OwnerReferences, Finalizers) from the original
// current object.
func DefaultFieldApplicator(current, desired *autoscalingv2.HorizontalPodAutoscaler) error {
	original := current.DeepCopy()
	*current = *desired.DeepCopy()
	generic.PreserveServerManagedFields(current, original)
	return nil
}

// Resource is a high-level abstraction for managing a Kubernetes HorizontalPodAutoscaler
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.Operational: for reporting operational status based on HPA conditions.
//   - component.Suspendable: for no-op suspend behaviour (idle HPA has no cluster impact).
//   - component.DataExtractable: for exporting values after successful reconciliation.
type Resource struct {
	base *generic.IntegrationResource[*autoscalingv2.HorizontalPodAutoscaler, *Mutator]
}

// Identity returns a unique identifier for the HPA in the format
// "autoscaling/v2/HorizontalPodAutoscaler/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes HorizontalPodAutoscaler object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes HorizontalPodAutoscaler into the desired state.
//
// The mutation process follows this order:
//  1. Field application: the current object is updated to reflect the desired base state,
//     using either DefaultFieldApplicator or a custom applicator if one is configured.
//  2. Field application flavors: any registered flavors are applied in registration order.
//  3. Feature mutations: all registered feature-gated mutations are applied in order.
//
// This method is invoked by the framework during the Update phase of reconciliation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus reports the HPA's operational status using the configured handler.
//
// By default, it uses DefaultOperationalStatusHandler, which inspects HPA conditions
// to determine if the autoscaler is active, pending, or failing.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// DeleteOnSuspend determines whether the HPA should be deleted from the cluster
// when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns false. The HPA
// is left in place to avoid unnecessary churn and simplify resumption. Note that
// a retained HPA may attempt to scale its target if the target still exists and is
// scaled to zero; override with WithCustomSuspendDeletionDecision if this is a concern.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend registers the configured suspension mutation for the next mutate cycle.
//
// For HPA, the default suspension mutation is a no-op since an idle HPA has no
// impact on the cluster.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus reports the suspension status of the HPA.
//
// By default, it uses DefaultSuspensionStatusHandler, which reports Suspended
// immediately because the default suspend is a no-op.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled HPA.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the HPA.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
