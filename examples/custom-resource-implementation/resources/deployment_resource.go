package resources

import (
	"fmt"
	"maps"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// DeploymentResource is a wrapper around a Kubernetes Deployment object.
// It implements the component.Resource, component.Alive, component.Suspendable,
// and component.DataExtractable interfaces from the Component Framework.
// This abstraction exists to decouple the reconciliation logic from the specific
// Kubernetes type, allowing the framework to handle lifecycle and status aggregation.
type DeploymentResource struct {
	// deployment becomes the underlying Kubernetes object after a reconcile and is
	// the desired core object before a reconcile (more concretely before a successful return from Mutate).
	deployment *appsv1.Deployment
	// mutations is a list of feature-gated changes to apply to the resource.
	mutations []feature.Mutation[*DeploymentResourceMutator]

	// convergeStatusHandler allows injecting custom logic for determining if the resource is ready.
	convergeStatusHandler func(concepts.ConvergingOperation, *appsv1.Deployment) (concepts.AliveStatusWithReason, error)
	// graceStatusHandler allows injecting custom logic for determining health when not fully ready.
	graceStatusHandler func(*appsv1.Deployment) (concepts.GraceStatusWithReason, error)
	// suspendStatusHandler allows injecting custom logic for determining if the resource is suspended.
	suspendStatusHandler func(*appsv1.Deployment) (concepts.SuspensionStatusWithReason, error)
	// suspendMutationHandler allows injecting custom logic for how to suspend the resource.
	suspendMutationHandler func(*appsv1.Deployment) error
	// suspendDeletionDecisionHandler allows injecting custom logic for whether to delete on suspension.
	suspendDeletionDecisionHandler func(*appsv1.Deployment) bool

	// suspender is a deferred mutation applied during Mutate() to handle suspension.
	suspender func(*appsv1.Deployment) error

	// dataExtractors are functions that pull information from the reconciled resource.
	dataExtractors []func(appsv1.Deployment) error
}

// Identity returns a unique identifier for the resource, used by the framework for logging and tracking.
func (r *DeploymentResource) Identity() string {
	return fmt.Sprintf("apps/v1/Deployment/%s", r.deployment.Name)
}

// Object returns a copy of the underlying Kubernetes object after reconcile or the unchanged desired object
// before a reconcile happens. It implements the Resource interface.
func (r *DeploymentResource) Object() (client.Object, error) {
	return r.deployment.DeepCopy(), nil
}

// Mutate applies the desired state to the resource, including Feature Mutations.
// It demonstrates how the Mutator pattern is used to safely apply version-gated changes.
func (r *DeploymentResource) Mutate(current client.Object) error {
	currentDeployment, ok := current.(*appsv1.Deployment)
	if !ok {
		return fmt.Errorf("expected *appsv1.Deployment, got %T", current)
	}

	// 1. Apply core desired state to the current object.
	// This ensures that the base fields are always correct before features apply their changes.
	r.applyCoreDesiredState(currentDeployment)

	// 2. Apply feature mutations via a restricted mutator interface
	// We've applied all desired fields from the core object and can now continue working
	// on mutations against the current object exclusively.
	mutator := NewDeploymentResourceMutator(currentDeployment)

	for _, m := range r.mutations {
		// Apply the **intent** of each mutator
		if err := m.ApplyIntent(mutator); err != nil {
			return fmt.Errorf("failed to apply mutation intent for %s: %w", m.Name, err)
		}
	}

	// Apply all gathered mutations using the mutator
	if err := mutator.Apply(); err != nil {
		return fmt.Errorf("failed to apply planned mutations: %w", err)
	}

	// 3. Apply a deferred suspension mutation if one was requested.
	if r.suspender != nil {
		if err := r.suspender(currentDeployment); err != nil {
			return err
		}
	}

	// 4. Update internal desired state with the mutated current object.
	// This ensures that subsequent calls to ConvergingStatus and ExtractData
	// use the fully mutated state, including status.
	r.deployment = currentDeployment

	return nil
}

// applyCoreDesiredState applies the core fields from the desired to the current object.
// It only applies fields that are considered mutable and should be managed by the component baseline.
func (r *DeploymentResource) applyCoreDesiredState(current *appsv1.Deployment) {
	if current == nil {
		return
	}

	// Apply labels and annotations
	if current.Labels == nil {
		current.Labels = make(map[string]string)
	}
	maps.Copy(current.Labels, r.deployment.Labels)

	if current.Annotations == nil {
		current.Annotations = make(map[string]string)
	}
	maps.Copy(current.Annotations, r.deployment.Annotations)

	// Apply Spec fields
	current.Spec.Replicas = r.deployment.Spec.Replicas
	current.Spec.Selector = r.deployment.Spec.Selector
	current.Spec.Template.Labels = r.deployment.Spec.Template.Labels
	current.Spec.Template.Annotations = r.deployment.Spec.Template.Annotations

	// For containers, we might want a more sophisticated merge, but for this example
	// we just ensure the core containers exist with their base settings.
	// This is where the "core" part of the resource definition is enforced.
	current.Spec.Template.Spec.Containers = make([]corev1.Container, len(r.deployment.Spec.Template.Spec.Containers))
	copy(current.Spec.Template.Spec.Containers, r.deployment.Spec.Template.Spec.Containers)
}

// ConvergingStatus reports the progress of the resource toward its desired state.
// It implements the Alive interface, allowing the Component to aggregate status.
func (r *DeploymentResource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.AliveStatusWithReason, error) {
	if r.convergeStatusHandler != nil {
		return r.convergeStatusHandler(op, r.deployment)
	}

	desiredReplicas := int32(1)
	if r.deployment.Spec.Replicas != nil {
		desiredReplicas = *r.deployment.Spec.Replicas
	}

	if r.deployment.Status.ReadyReplicas == desiredReplicas {
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusHealthy,
			Reason: "All replicas are ready",
		}, nil
	}

	var status concepts.AliveConvergingStatus
	switch op {
	case concepts.ConvergingOperationCreated:
		status = concepts.AliveConvergingStatusCreating
	case concepts.ConvergingOperationUpdated:
		status = concepts.AliveConvergingStatusUpdating
	default:
		status = concepts.AliveConvergingStatusScaling
	}

	return concepts.AliveStatusWithReason{
		Status: status,
		Reason: fmt.Sprintf(
			"Waiting for replicas: %d/%d ready",
			r.deployment.Status.ReadyReplicas,
			desiredReplicas,
		),
	}, nil
}

// GraceStatus reports the health of the resource when it's not fully ready (e.g., degraded).
// It's part of the Alive interface used for sophisticated status reporting.
func (r *DeploymentResource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	if r.graceStatusHandler != nil {
		return r.graceStatusHandler(r.deployment)
	}

	if r.deployment.Status.ReadyReplicas > 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Deployment partially available",
		}, nil
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDown,
		Reason: "No replicas are ready",
	}, nil
}

// DeleteOnSuspend determines if the resource should be deleted when the component is suspended.
func (r *DeploymentResource) DeleteOnSuspend() bool {
	if r.suspendDeletionDecisionHandler != nil {
		return r.suspendDeletionDecisionHandler(r.deployment)
	}
	return false
}

// Suspend records the intent to suspend the resource, to be applied during the next reconcile.
func (r *DeploymentResource) Suspend() error {
	// Suspension intent is recorded here and applied later in Mutate().
	// This keeps all desired-state mutation in one place.
	if r.suspendMutationHandler != nil {
		r.suspender = func(obj *appsv1.Deployment) error {
			defer func() { r.suspender = nil }()
			return r.suspendMutationHandler(obj)
		}
		return nil
	}

	r.suspender = func(obj *appsv1.Deployment) error {
		defer func() { r.suspender = nil }()
		obj.Spec.Replicas = ptr.To(int32(0))
		return nil
	}

	return nil
}

// SuspensionStatus reports the progress of the resource toward a suspended state.
func (r *DeploymentResource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	if r.suspendStatusHandler != nil {
		return r.suspendStatusHandler(r.deployment)
	}

	if r.deployment.Status.Replicas == 0 {
		return concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspended,
			Reason: "Deployment scaled to zero",
		}, nil
	}

	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspending,
		Reason: fmt.Sprintf(
			"Waiting for replicas to scale down, %d replicas still running.",
			r.deployment.Status.Replicas,
		),
	}, nil
}

// ExtractData pulls information from the reconciled resource for use by other components.
// It implements the DataExtractable interface.
func (r *DeploymentResource) ExtractData() error {
	// We enure no data mutations are applied by extractors
	deploymentCopy := r.deployment.DeepCopy()

	for _, extractor := range r.dataExtractors {
		if extractor == nil {
			continue
		}

		if err := extractor(*deploymentCopy); err != nil {
			return err
		}
	}

	return nil
}
