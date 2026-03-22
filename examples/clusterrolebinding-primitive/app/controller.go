// Package app provides a sample controller using the clusterrolebinding primitive.
//
// Note: ClusterRoleBinding is cluster-scoped. The component framework sets a
// controller owner reference, which requires the owner to also be cluster-scoped.
// In production, use a cluster-scoped CRD as the owner. This example demonstrates
// the controller pattern for reference; the main.go entry point exercises the
// primitive API directly to avoid the cluster-scoped owner requirement.
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
// In production usage with a cluster-scoped resource, the owner (ExampleApp) should
// also be cluster-scoped to allow controller owner references to be set correctly.
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
