// Package app provides a sample controller demonstrating mutations and gating.
package app

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles an ExampleApp by managing a Deployment and a ConfigMap
// within a single component. The ConfigMap is gated on EnableMetrics, so it is
// created only when metrics are enabled and deleted when they are disabled.
type Controller struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder events.EventRecorder
	Metrics       component.MetricsRecorder

	NewDeploymentResource func(*ExampleApp) (component.Resource, error)
	NewConfigMapResource  func(*ExampleApp) (component.Resource, error)
}

// Reconcile builds and reconciles a single component containing both resources.
func (r *Controller) Reconcile(ctx context.Context, owner *ExampleApp) (err error) {
	recCtx := component.ReconcileContext{
		Client:        r.Client,
		Scheme:        r.Scheme,
		EventRecorder: r.EventRecorder,
		Metrics:       r.Metrics,
		Owner:         owner,
	}
	defer func() {
		if flushErr := component.FlushStatus(ctx, recCtx); flushErr != nil && err == nil {
			err = flushErr
		}
	}()

	comp, err := BuildComponent(owner, r.NewDeploymentResource, r.NewConfigMapResource)
	if err != nil {
		return err
	}

	return comp.Reconcile(ctx, recCtx)
}

// BuildComponent assembles the reconciled component for the given owner from the
// supplied resource factories. It is the single source of truth for how the
// Deployment and ConfigMap are composed into one component, shared by the
// controller's reconcile path and by component-level golden tests so both
// exercise the exact same assembly.
//
// The factories are passed in rather than imported so this package stays free of
// a dependency on the resources package (which already imports this one). The
// controller injects its production factories; tests pass the same ones via
// resources.NewDeploymentResource / resources.NewConfigMapResource.
//
// The ConfigMap is gated at the resource level: when metrics are disabled the
// framework deletes it.
func BuildComponent(
	owner *ExampleApp,
	newDeployment func(*ExampleApp) (component.Resource, error),
	newConfigMap func(*ExampleApp) (component.Resource, error),
) (*component.Component, error) {
	deployResource, err := newDeployment(owner)
	if err != nil {
		return nil, err
	}

	cmResource, err := newConfigMap(owner)
	if err != nil {
		return nil, err
	}

	return component.NewComponentBuilder().
		WithName("example-app").
		WithConditionType("AppReady").
		WithResource(deployResource).
		WithResource(cmResource, component.DeleteWhen(!owner.Spec.EnableMetrics)).
		Suspend(owner.Spec.Suspended).
		Build()
}
