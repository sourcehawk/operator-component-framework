// Package app provides a sample controller using the clusterrolebinding primitive.
//
// Note: ClusterRoleBinding is cluster-scoped. When the owner is namespace-scoped,
// the component framework automatically skips setting a controller owner reference
// (since Kubernetes does not allow cross-scope owner references) and logs an info
// message. This means the ClusterRoleBinding will not be garbage-collected when
// the owner is deleted — operators should implement their own cleanup logic if
// needed. In production, using a cluster-scoped CRD as the owner avoids this
// limitation entirely.
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
//
// When the owner is namespace-scoped, the framework skips the controller owner
// reference for cluster-scoped resources. Use a cluster-scoped owner CRD in
// production if automatic garbage collection is required.
type ExampleController struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder

	// NewClusterRoleBindingResource is a factory function to create the clusterrolebinding resource.
	NewClusterRoleBindingResource func(*sharedapp.ExampleApp) (component.Resource, error)
}

// Reconcile performs the reconciliation for a single ExampleApp.
func (r *ExampleController) Reconcile(ctx context.Context, owner *sharedapp.ExampleApp) error {
	// 1. Build the clusterrolebinding resource for this owner.
	crbResource, err := r.NewClusterRoleBindingResource(owner)
	if err != nil {
		return err
	}

	// 2. Build the component that manages the clusterrolebinding.
	comp, err := component.NewComponentBuilder().
		WithName("example-app").
		WithConditionType("AppReady").
		WithResource(crbResource, component.ResourceOptions{}).
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
