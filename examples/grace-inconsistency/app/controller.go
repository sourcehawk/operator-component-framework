// Package app provides a sample controller demonstrating grace inconsistency suppression.
package app

import (
	"context"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles an ExampleApp with a Deployment that has a custom grace
// handler intentionally returning Healthy while the convergence handler may
// report non-healthy. The inconsistency warning is suppressed via ResourceOptions.
type Controller struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder

	NewDeploymentResource func(*ExampleApp) (component.Resource, error)
}

// Reconcile builds and reconciles a component with grace period and
// inconsistency suppression.
func (r *Controller) Reconcile(ctx context.Context, owner *ExampleApp) error {
	deployResource, err := r.NewDeploymentResource(owner)
	if err != nil {
		return err
	}

	// SuppressGraceInconsistencyWarning tells the framework not to log a
	// warning when the custom grace handler reports Healthy while the
	// convergence handler reports non-healthy. This is intentional: the
	// deployment is a soft dependency and should not block the component.
	opts, err := component.NewResourceOptionsBuilder().
		SuppressGraceInconsistencyWarning().
		Build()
	if err != nil {
		return err
	}

	comp, err := component.NewComponentBuilder().
		WithName("monitoring").
		WithConditionType("MonitoringReady").
		WithResource(deployResource, opts).
		WithGracePeriod(5 * time.Second).
		Build()
	if err != nil {
		return err
	}

	return comp.Reconcile(ctx, component.ReconcileContext{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Recorder: r.Recorder,
		Metrics:  r.Metrics,
		Owner:    owner,
	})
}
