// Package app provides a sample controller demonstrating custom resource management
// via the unstructured static builder.
package app

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles an ExampleApp by managing a CertificateRequest custom
// resource using the unstructured static builder.
type Controller struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder events.EventRecorder
	Metrics       component.MetricsRecorder
	// APIReader reads straight from the API server; FlushStatus uses it on a
	// conflict so the retry sees the live owner rather than a stale cache entry.
	APIReader client.Reader

	NewCertificateResource func(*ExampleApp) (component.Resource, error)
}

// Reconcile builds and reconciles a single component managing the certificate.
func (r *Controller) Reconcile(ctx context.Context, owner *ExampleApp) (err error) {
	recCtx := component.ReconcileContext{
		Client:        r.Client,
		Scheme:        r.Scheme,
		EventRecorder: r.EventRecorder,
		Metrics:       r.Metrics,
		APIReader:     r.APIReader,
		Owner:         owner,
	}
	// Declared before the deferred flush so the closure sees every component
	// that gets built below. FlushStatus derives the condition types it owns
	// from these: on a conflict the owned conditions stay on this staged owner
	// while the unowned ones are refreshed from the server.
	var comps []*component.Component
	defer func() {
		if flushErr := component.FlushStatus(ctx, recCtx, comps); flushErr != nil && err == nil {
			err = flushErr
		}
	}()

	certResource, err := r.NewCertificateResource(owner)
	if err != nil {
		return err
	}

	comp, err := component.NewComponentBuilder().
		WithName("certificate").
		WithConditionType("CertificateReady").
		WithResource(certResource).
		Build()
	if err != nil {
		return err
	}
	comps = []*component.Component{comp}

	return comp.Reconcile(ctx, recCtx)
}
