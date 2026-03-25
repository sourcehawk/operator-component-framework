// Package app provides a sample controller using the cronjob primitive.
package app

import (
	"context"

	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ExampleApp re-exports the shared CRD type so callers in this package need no import alias.
type ExampleApp = sharedapp.ExampleApp

// ExampleAppSpec re-exports the shared spec type.
type ExampleAppSpec = sharedapp.ExampleAppSpec

// ExampleAppStatus re-exports the shared status type.
type ExampleAppStatus = sharedapp.ExampleAppStatus

// ExampleAppList re-exports the shared list type.
type ExampleAppList = sharedapp.ExampleAppList

// AddToScheme registers the ExampleApp types with the given scheme.
var AddToScheme = sharedapp.AddToScheme

// ExampleController reconciles an ExampleApp object using the component framework.
type ExampleController struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder

	// NewCronJobResource is a factory function to create the cronjob resource.
	NewCronJobResource func(*ExampleApp) (component.Resource, error)
}

// Reconcile performs the reconciliation for a single ExampleApp.
func (r *ExampleController) Reconcile(ctx context.Context, owner *ExampleApp) error {
	// 1. Build the cronjob resource for this owner.
	cronJobResource, err := r.NewCronJobResource(owner)
	if err != nil {
		return err
	}

	// 2. Build the component that manages the cronjob.
	comp, err := component.NewComponentBuilder().
		WithName("example-cronjob").
		WithConditionType("CronJobReady").
		WithResource(cronJobResource, component.ResourceOptions{}).
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
