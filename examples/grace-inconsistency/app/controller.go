// Package app provides a sample controller demonstrating grace inconsistency suppression.
package app

import (
	"context"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles an ExampleApp with a Deployment that has a custom grace
// handler intentionally returning Healthy while the convergence handler may
// report non-healthy. The inconsistency warning is suppressed via the
// SuppressGraceInconsistencyWarning() option.
type Controller struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder events.EventRecorder
	Metrics       component.MetricsRecorder
	// APIReader reads straight from the API server; FlushStatus uses it on a
	// conflict so the retry sees the live owner rather than a stale cache entry.
	APIReader client.Reader

	NewDeploymentResource func(*ExampleApp) (component.Resource, error)
}

// Reconcile builds and reconciles a component with grace period and
// inconsistency suppression.
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

	comp, err := r.BuildComponent(owner)
	if err != nil {
		return err
	}
	comps = []*component.Component{comp}

	return comp.Reconcile(ctx, recCtx)
}

// BuildComponent assembles the monitoring component: a Deployment whose custom
// grace handler reports Healthy while the convergence handler may report
// non-healthy. The grace period and the SuppressGraceInconsistencyWarning option
// are the point of this example, so the controller and tests build the component
// the same way to keep the reconciled component and the golden snapshot in
// lockstep.
func (r *Controller) BuildComponent(owner *ExampleApp) (*component.Component, error) {
	deployResource, err := r.NewDeploymentResource(owner)
	if err != nil {
		return nil, err
	}

	return component.NewComponentBuilder().
		WithName("monitoring").
		WithConditionType("MonitoringReady").
		// SuppressGraceInconsistencyWarning tells the framework not to log a
		// warning when the custom grace handler reports Healthy while the
		// convergence handler reports non-healthy. This is intentional: the
		// deployment is a soft dependency and should not block the component.
		WithResource(deployResource, component.SuppressGraceInconsistencyWarning()).
		WithGracePeriod(5 * time.Second).
		Build()
}
