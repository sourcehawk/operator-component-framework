// Package app provides a sample controller using the serviceaccount primitive.
package app

import (
	"context"

	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
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

	// NewServiceAccountResource is a factory function to create the serviceaccount resource.
	// This allows us to inject the resource construction logic.
	NewServiceAccountResource func(*sharedapp.ExampleApp) (component.Resource, error)
}

// Reconcile performs the reconciliation for a single ExampleApp.
func (r *ExampleController) Reconcile(ctx context.Context, owner *sharedapp.ExampleApp) error {
	// 1. Build the serviceaccount resource for this owner.
	saResource, err := r.NewServiceAccountResource(owner)
	if err != nil {
		return err
	}

	// 2. Build the component that manages the serviceaccount.
	comp, err := component.NewComponentBuilder().
		WithName("example-app").
		WithConditionType("AppReady").
		WithResource(saResource, component.ResourceOptions{}).
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
