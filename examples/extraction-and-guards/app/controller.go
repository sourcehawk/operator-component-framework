// Package app provides a sample controller demonstrating data extraction and guards.
package app

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles an ExampleApp by managing a ConfigMap and a Secret
// within a single component. The ConfigMap exposes data via a declared
// extraction, and the Secret is guarded until that data is available.
type Controller struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder events.EventRecorder
	Metrics       component.MetricsRecorder

	// NewConfigMapResource builds the ConfigMap and declares the extraction
	// that writes the dbHost cell.
	NewConfigMapResource func(owner *ExampleApp, dbHost *concepts.Data[string]) (component.Resource, error)

	// NewSecretResource builds the Secret with a data guard and a mutation
	// that read the dbHost cell.
	NewSecretResource func(owner *ExampleApp, dbHost *concepts.Data[string]) (component.Resource, error)
}

// Reconcile builds and reconciles a component where the ConfigMap is registered
// before the Secret. Registration order matters: the guard on the Secret can
// only read data extracted by a preceding resource.
func (r *Controller) Reconcile(ctx context.Context, owner *ExampleApp) (err error) {
	recCtx := component.ReconcileContext{
		Client:        r.Client,
		Scheme:        r.Scheme,
		EventRecorder: r.EventRecorder,
		Metrics:       r.Metrics,
		Owner:         owner,
	}
	// Declared before the deferred flush so the closure sees every component
	// that gets built below. FlushStatus derives the condition types it owns
	// from these, and only those are re-applied over the server's copy after a
	// conflict.
	var comps []*component.Component
	defer func() {
		if flushErr := component.FlushStatus(ctx, recCtx, comps); flushErr != nil && err == nil {
			err = flushErr
		}
	}()

	comp, _, err := r.BuildComponent(owner)
	if err != nil {
		return err
	}
	comps = []*component.Component{comp}

	return comp.Reconcile(ctx, recCtx)
}

// BuildComponent assembles the database component: a ConfigMap registered
// before a Secret, both wired to a shared data cell. The ConfigMap's declared
// extraction writes the cell; the Secret's data guard and mutation read it.
// Build() verifies the ordering. The cell is returned so tests can seed it
// when rendering cluster-free previews and assert the declared topology.
func (r *Controller) BuildComponent(owner *ExampleApp) (*component.Component, *concepts.Data[string], error) {
	dbHost := concepts.NewData[string]("db-host")

	cmResource, err := r.NewConfigMapResource(owner, dbHost)
	if err != nil {
		return nil, nil, err
	}

	secretResource, err := r.NewSecretResource(owner, dbHost)
	if err != nil {
		return nil, nil, err
	}

	comp, err := component.NewComponentBuilder().
		WithName("database").
		WithConditionType("DatabaseReady").
		WithResource(cmResource).
		WithResource(secretResource).
		Build()
	if err != nil {
		return nil, nil, err
	}
	return comp, dbHost, nil
}
