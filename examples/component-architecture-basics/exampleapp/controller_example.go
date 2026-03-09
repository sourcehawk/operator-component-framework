// Package exampleapp provides a sample controller implementation using the component framework.
package exampleapp

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/sourcehawk/operator-component-framework/examples/component-architecture-basics/features"
	"github.com/sourcehawk/operator-component-framework/examples/component-architecture-basics/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
)

// ExampleController is a mock controller demonstrating the Component framework's usage.
// It orchestrates resource creation with Feature Mutations and uses the
// ComponentBuilder to assemble a behavioral unit for reconciliation.
type ExampleController struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder
}

// Reconcile performs the component-based reconciliation flow.
// It shows how to:
// 1. Configure resources with version-aware Feature Mutations.
// 2. Assemble those resources into a named Component.
// 3. Use ReconcileContext to execute standardized framework reconciliation.
func (r *ExampleController) Reconcile(ctx context.Context, owner *ExamplePlatform) error {
	// 1. Build the resource with features based on owner version.
	// We use the owner's name as a prefix to ensure uniqueness in our multi-scenario example.
	resourceName := owner.Name + "-web-ui"
	res := resources.NewDeploymentBuilder(resourceName, owner.Namespace).
		WithMutation(features.NewTracingFeature(owner.Spec.Version, owner.Spec.EnableTracing)).
		WithMutation(features.NewLegacyCompatibilityFeature(owner.Spec.Version)).
		WithMutation(features.NewLegacyBehaviorFeature(owner.Spec.Version)).
		WithDataExtractor(func(d appsv1.Deployment) error {
			return debugPrinterAsDataExtractor(ctx, d)
		}).
		Build()

	// 2. Assemble the component using the real framework builder
	comp, err := component.NewComponentBuilder(owner.Spec.Suspended).
		WithName("web-interface").
		WithConditionType("WebInterfaceReady").
		WithResource(res, false, false).
		Build()

	if err != nil {
		return err
	}

	// 3. Reconcile the component using the real ReconcileContext
	reconCtx := component.ReconcileContext{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Recorder: r.Recorder,
		Metrics:  r.Metrics,
		Owner:    owner,
	}

	err = comp.Reconcile(ctx, reconCtx)
	if err != nil {
		return err
	}

	return nil
}

// debugPrinterAsDataExtractor An example data extractor that does not actually extract anything but
// prints debug logs for us in the example.
func debugPrinterAsDataExtractor(ctx context.Context, d appsv1.Deployment) error {
	logger := log.FromContext(ctx)

	logger.Info("Deployment State", "name", d.Name)
	if d.Spec.Replicas != nil {
		logger.Info("  Replicas", "count", *d.Spec.Replicas)
	}
	logger.Info("  Labels", "labels", d.Labels)
	for _, c := range d.Spec.Template.Spec.Containers {
		logger.Info("  Container", "name", c.Name)
		logger.Info("    Args", "args", c.Args)
		logger.Info("    Env", "env", c.Env)
	}
	return nil
}
