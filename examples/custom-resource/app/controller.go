// Package app provides a sample controller demonstrating custom resource management
// via the unstructured static builder.
package app

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles an ExampleApp by managing a CertificateRequest custom
// resource using the unstructured static builder.
type Controller struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  component.Recorder

	NewCertificateResource func(*ExampleApp) (component.Resource, error)
}

// Reconcile builds and reconciles a single component managing the certificate.
func (r *Controller) Reconcile(ctx context.Context, owner *ExampleApp) error {
	certResource, err := r.NewCertificateResource(owner)
	if err != nil {
		return err
	}

	comp, err := component.NewComponentBuilder().
		WithName("certificate").
		WithConditionType("CertificateReady").
		WithResource(certResource, component.ResourceOptions{}).
		Build()
	if err != nil {
		return err
	}

	return comp.Reconcile(ctx, component.ReconcileContext{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Recorder: r.Recorder,
		Metrics:  r.Metrics,
		Owner:    owner,
	})
}
