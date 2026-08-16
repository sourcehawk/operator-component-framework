package framework

import (
	"context"
	"sync"

	"github.com/sourcehawk/operator-component-framework/pkg/component"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ResourceFactory builds a single component.Resource from the owner TestApp.
// The reconciler wraps the returned resource in a single-resource Component
// with condition type "E2EReady".
type ResourceFactory func(owner *TestApp) (component.Resource, error)

// ComponentFactory builds a full *component.Component from the owner TestApp.
// Use this for multi-resource component tests where you need full control
// over the component configuration.
type ComponentFactory func(owner *TestApp) (*component.Component, error)

// E2EReconciler is a controller-runtime reconciler that delegates component
// construction to per-test factories registered by namespace/name key.
type E2EReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder events.EventRecorder
	Metrics       component.MetricsRecorder

	mu                 sync.RWMutex
	resourceFactories  map[string]ResourceFactory
	componentFactories map[string]ComponentFactory
}

// NewE2EReconciler creates a new E2EReconciler.
func NewE2EReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	recorder events.EventRecorder,
	metrics component.MetricsRecorder,
) *E2EReconciler {
	return &E2EReconciler{
		Client:             c,
		Scheme:             scheme,
		EventRecorder:      recorder,
		Metrics:            metrics,
		resourceFactories:  make(map[string]ResourceFactory),
		componentFactories: make(map[string]ComponentFactory),
	}
}

// RegisterResource registers a ResourceFactory for a specific TestApp identified
// by its namespaced name. The factory is called on each reconciliation to build
// the resource, which is wrapped in a single-resource Component.
func (r *E2EReconciler) RegisterResource(key types.NamespacedName, factory ResourceFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resourceFactories[key.String()] = factory
}

// RegisterComponent registers a ComponentFactory for a specific TestApp identified
// by its namespaced name. The factory is called on each reconciliation to build
// the full Component directly.
func (r *E2EReconciler) RegisterComponent(key types.NamespacedName, factory ComponentFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.componentFactories[key.String()] = factory
}

// Unregister removes any registered factory for the given namespaced name.
func (r *E2EReconciler) Unregister(key types.NamespacedName) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.resourceFactories, key.String())
	delete(r.componentFactories, key.String())
}

// Reconcile implements reconcile.Reconciler. It fetches the TestApp, looks up
// the registered factory, builds the component, and reconciles it.
func (r *E2EReconciler) Reconcile(ctx context.Context, req reconcile.Request) (_ reconcile.Result, err error) {
	logger := log.FromContext(ctx).WithValues("testapp", req.NamespacedName)

	owner := &TestApp{}
	if err := r.Get(ctx, req.NamespacedName, owner); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	key := req.String()

	r.mu.RLock()
	compFactory, hasComp := r.componentFactories[key]
	resFactory, hasRes := r.resourceFactories[key]
	r.mu.RUnlock()

	var comp *component.Component

	switch {
	case hasComp:
		comp, err = compFactory(owner)
	case hasRes:
		resource, buildErr := resFactory(owner)
		if buildErr != nil {
			return reconcile.Result{}, buildErr
		}
		comp, err = component.NewComponentBuilder().
			WithName("e2e-test").
			WithConditionType("E2EReady").
			WithResource(resource).
			Suspend(owner.Spec.Suspended).
			Build()
	default:
		logger.V(1).Info("no factory registered, skipping")
		return reconcile.Result{}, nil
	}

	if err != nil {
		return reconcile.Result{}, err
	}

	recCtx := component.ReconcileContext{
		Client:        r.Client,
		Scheme:        r.Scheme,
		EventRecorder: r.EventRecorder,
		Metrics:       r.Metrics,
		Owner:         owner,
	}
	defer func() {
		if flushErr := component.FlushStatus(ctx, recCtx, comp); flushErr != nil && err == nil {
			err = flushErr
		}
	}()

	if err := comp.Reconcile(ctx, recCtx); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
