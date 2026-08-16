// Package app provides a sample controller demonstrating component-level prerequisites.
package app

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
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
	Scheme        *runtime.Scheme
	EventRecorder events.EventRecorder
	Metrics       component.MetricsRecorder
	// APIReader reads straight from the API server; FlushStatus uses it on a
	// conflict so the retry sees the live owner rather than a stale cache entry.
	APIReader client.Reader

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
		Client:        r.Client,
		Scheme:        r.Scheme,
		EventRecorder: r.EventRecorder,
		Metrics:       r.Metrics,
		APIReader:     r.APIReader,
		Owner:         owner,
	}
	// Declared before the deferred flush so the closure sees every component
	// that gets built below. FlushStatus derives the condition types it owns
	// from these: on a conflict the owned conditions stay on this staged owner
	// while the unowned ones are refreshed from the server.
	var comps []*component.Component
	defer func() {
		if flushErr := component.FlushStatus(ctx, recCtx, comps); flushErr != nil && err == nil {
			err = flushErr
		}
	}()

	// --- Infra component: no prerequisites ---
	infra, err := r.BuildInfraComponent(owner)
	if err != nil {
		return err
	}
	comps = append(comps, infra)

	if err := infra.Reconcile(ctx, recCtx); err != nil {
		return err
	}

	// --- App component: depends on InfraReady ---
	app, err := r.BuildAppComponent(owner)
	if err != nil {
		return err
	}
	comps = append(comps, app)

	return app.Reconcile(ctx, recCtx)
}

// BuildInfraComponent assembles the infra component: a single ConfigMap reporting
// the InfraReady condition, with no prerequisites. The controller and tests share
// this so the reconciled component and the golden snapshot stay in lockstep.
func (r *Controller) BuildInfraComponent(owner *ExampleApp) (*component.Component, error) {
	cmResource, err := r.NewConfigMapResource(owner)
	if err != nil {
		return nil, err
	}

	return component.NewComponentBuilder().
		WithName("infra").
		WithConditionType("InfraReady").
		WithResource(cmResource).
		Build()
}

// BuildAppComponent assembles the app component: a Deployment reporting the
// AppReady condition, gated behind the InfraReady prerequisite. The DependsOn
// prerequisite is the point of this example, so the controller and tests build
// the component the same way.
func (r *Controller) BuildAppComponent(owner *ExampleApp) (*component.Component, error) {
	deployResource, err := r.NewDeploymentResource(owner)
	if err != nil {
		return nil, err
	}

	return component.NewComponentBuilder().
		WithName("app").
		WithConditionType("AppReady").
		WithResource(deployResource).
		WithPrerequisite(component.DependsOn("InfraReady")).
		Suspend(owner.Spec.Suspended).
		Build()
}
