// Package app provides a sample controller demonstrating component-level prerequisites.
package app

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles an ExampleApp using two components:
//   - "infra" manages a ConfigMap and reports the InfraReady condition.
//   - "app" manages a Deployment and depends on InfraReady via a prerequisite.
//
// The controller reconciles both components in sequence. The app component
// will not proceed until the infra component's condition is True.
type Controller struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder

	NewConfigMapResource  func(*ExampleApp) (component.Resource, error)
	NewDeploymentResource func(*ExampleApp) (component.Resource, error)
}

// Reconcile builds and reconciles the infra and app components in order.
//
// Both components share the same ReconcileContext and stage their conditions
// on the owner in memory; a single deferred FlushStatus at the end of
// reconciliation persists both conditions in one API call. That is what
// prevents the sequential components from racing two separate status updates
// against the same owner and hitting conflicts.
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

	// --- Infra component: no prerequisites ---
	cmResource, err := r.NewConfigMapResource(owner)
	if err != nil {
		return err
	}

	infra, err := component.NewComponentBuilder().
		WithName("infra").
		WithConditionType("InfraReady").
		WithResource(cmResource).
		Build()
	if err != nil {
		return err
	}

	if err := infra.Reconcile(ctx, recCtx); err != nil {
		return err
	}

	// --- App component: depends on InfraReady ---
	deployResource, err := r.NewDeploymentResource(owner)
	if err != nil {
		return err
	}

	app, err := component.NewComponentBuilder().
		WithName("app").
		WithConditionType("AppReady").
		WithResource(deployResource).
		WithPrerequisite(component.DependsOn("InfraReady")).
		Suspend(owner.Spec.Suspended).
		Build()
	if err != nil {
		return err
	}

	return app.Reconcile(ctx, recCtx)
}
