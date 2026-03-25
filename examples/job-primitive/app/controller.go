// Package app provides a sample controller using the job primitive.
package app

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ExampleController reconciles an ExampleApp object using the component framework.
type ExampleController struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder

	// NewJobResource is a factory function to create the job resource.
	// This allows us to inject the resource construction logic.
	NewJobResource func(*ExampleApp) (component.Resource, error)
}

// Reconcile performs the reconciliation for a single ExampleApp.
func (r *ExampleController) Reconcile(ctx context.Context, owner *ExampleApp) error {
	// 1. Build the job resource for this owner.
	jobResource, err := r.NewJobResource(owner)
	if err != nil {
		return err
	}

	// 2. Build the component that manages the job.
	comp, err := component.NewComponentBuilder().
		WithName("example-migration").
		WithConditionType("MigrationReady").
		WithResource(jobResource, component.ResourceOptions{}).
		Suspend(owner.Spec.Suspended).
		Build()
	if err != nil {
		return err
	}

	// 3. Execute the component reconciliation.
	resCtx := component.ReconcileContext{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Recorder: r.Recorder,
		Metrics:  r.Metrics,
		Owner:    owner,
	}

	return comp.Reconcile(ctx, resCtx)
}
