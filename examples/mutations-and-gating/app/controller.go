// Package app provides a sample controller demonstrating mutations and gating.
package app

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles an ExampleApp by managing a Deployment and a ConfigMap
// within a single component. The ConfigMap is gated on EnableMetrics, so it is
// created only when metrics are enabled and deleted when they are disabled.
type Controller struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder

	NewDeploymentResource func(*ExampleApp) (component.Resource, error)
	NewConfigMapResource  func(*ExampleApp) (component.Resource, error)
}

// Reconcile builds and reconciles a single component containing both resources.
func (r *Controller) Reconcile(ctx context.Context, owner *ExampleApp) (err error) {
	recCtx := component.ReconcileContext{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Recorder: r.Recorder,
		Metrics:  r.Metrics,
		Owner:    owner,
	}
	defer func() {
		if flushErr := component.FlushStatus(ctx, recCtx); flushErr != nil && err == nil {
			err = flushErr
		}
	}()

	deployResource, err := r.NewDeploymentResource(owner)
	if err != nil {
		return err
	}

	cmResource, err := r.NewConfigMapResource(owner)
	if err != nil {
		return err
	}

	// Gate the ConfigMap at the resource level: when metrics are disabled the
	// framework deletes the ConfigMap and excludes it from health aggregation.
	cmOpts, err := component.NewResourceOptionsBuilder().
		When(owner.Spec.EnableMetrics).
		Build()
	if err != nil {
		return err
	}

	comp, err := component.NewComponentBuilder().
		WithName("example-app").
		WithConditionType("AppReady").
		WithResource(deployResource, component.ResourceOptions{}).
		WithResource(cmResource, cmOpts).
		Suspend(owner.Spec.Suspended).
		Build()
	if err != nil {
		return err
	}

	return comp.Reconcile(ctx, recCtx)
}
