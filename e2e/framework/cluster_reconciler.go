package framework

import (
	"context"
	"sync"

	"github.com/sourcehawk/operator-component-framework/pkg/component"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ClusterResourceFactory builds a single component.Resource from the owner ClusterTestApp.
// The reconciler wraps the returned resource in a single-resource Component
// with condition type "E2EReady".
type ClusterResourceFactory func(owner *ClusterTestApp) (component.Resource, error)

// ClusterComponentFactory builds a full *component.Component from the owner ClusterTestApp.
// Use this for multi-resource component tests where you need full control
// over the component configuration.
type ClusterComponentFactory func(owner *ClusterTestApp) (*component.Component, error)

// ClusterE2EReconciler is a controller-runtime reconciler that delegates component
// construction to per-test factories registered by name key. It reconciles
// cluster-scoped ClusterTestApp resources for testing cluster-scoped primitives.
type ClusterE2EReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder

	mu                 sync.RWMutex
	resourceFactories  map[string]ClusterResourceFactory
	componentFactories map[string]ClusterComponentFactory
}

// NewClusterE2EReconciler creates a new ClusterE2EReconciler.
func NewClusterE2EReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	metrics component.Recorder,
) *ClusterE2EReconciler {
	return &ClusterE2EReconciler{
		Client:             c,
		Scheme:             scheme,
		Recorder:           recorder,
		Metrics:            metrics,
		resourceFactories:  make(map[string]ClusterResourceFactory),
		componentFactories: make(map[string]ClusterComponentFactory),
	}
}

// RegisterResource registers a ClusterResourceFactory for a specific ClusterTestApp
// identified by its name. The factory is called on each reconciliation to build
// the resource, which is wrapped in a single-resource Component.
func (r *ClusterE2EReconciler) RegisterResource(name string, factory ClusterResourceFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resourceFactories[name] = factory
}

// RegisterComponent registers a ClusterComponentFactory for a specific ClusterTestApp
// identified by its name. The factory is called on each reconciliation to build
// the full Component directly.
func (r *ClusterE2EReconciler) RegisterComponent(name string, factory ClusterComponentFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.componentFactories[name] = factory
}

// Unregister removes any registered factory for the given name.
func (r *ClusterE2EReconciler) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.resourceFactories, name)
	delete(r.componentFactories, name)
}

// Reconcile implements reconcile.Reconciler. It fetches the ClusterTestApp, looks up
// the registered factory, builds the component, and reconciles it.
func (r *ClusterE2EReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("clustertestapp", req.NamespacedName)

	owner := &ClusterTestApp{}
	if err := r.Get(ctx, req.NamespacedName, owner); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// ClusterTestApp is cluster-scoped so use the bare name as the key,
	// matching the string callers pass to RegisterResource/RegisterComponent.
	r.mu.RLock()
	compFactory, hasComp := r.componentFactories[owner.Name]
	resFactory, hasRes := r.resourceFactories[owner.Name]
	r.mu.RUnlock()

	var comp *component.Component
	var err error

	switch {
	case hasComp:
		comp, err = compFactory(owner)
	case hasRes:
		res, buildErr := resFactory(owner)
		if buildErr != nil {
			return reconcile.Result{}, buildErr
		}
		comp, err = component.NewComponentBuilder().
			WithName("e2e-test").
			WithConditionType("E2EReady").
			WithResource(res, component.ResourceOptions{}).
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
		Client:   r.Client,
		Scheme:   r.Scheme,
		Recorder: r.Recorder,
		Metrics:  r.Metrics,
		Owner:    owner,
	}

	if err := comp.Reconcile(ctx, recCtx); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
