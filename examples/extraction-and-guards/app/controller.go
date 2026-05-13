// Package app provides a sample controller demonstrating data extraction and guards.
package app

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles an ExampleApp by managing a ConfigMap and a Secret
// within a single component. The ConfigMap exposes data via extraction, and
// the Secret is guarded until that data is available.
type Controller struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder

	// NewConfigMapResource builds the ConfigMap and wires the data extractor.
	// The extractor writes to dbHost so the Secret guard can read it.
	NewConfigMapResource func(owner *ExampleApp, dbHost *string) (component.Resource, error)

	// NewSecretResource builds the Secret with a guard that reads dbHost.
	NewSecretResource func(owner *ExampleApp, dbHost *string) (component.Resource, error)
}

// Reconcile builds and reconciles a component where the ConfigMap is registered
// before the Secret. Registration order matters: the guard on the Secret can
// only read data extracted by a preceding resource.
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

	// Shared state: the ConfigMap extractor writes here, the Secret guard reads it.
	var dbHost string

	cmResource, err := r.NewConfigMapResource(owner, &dbHost)
	if err != nil {
		return err
	}

	secretResource, err := r.NewSecretResource(owner, &dbHost)
	if err != nil {
		return err
	}

	comp, err := component.NewComponentBuilder().
		WithName("database").
		WithConditionType("DatabaseReady").
		WithResource(cmResource, component.ResourceOptions{}).
		WithResource(secretResource, component.ResourceOptions{}).
		Build()
	if err != nil {
		return err
	}

	return comp.Reconcile(ctx, recCtx)
}
