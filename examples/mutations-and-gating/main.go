// Package main demonstrates mutations and resource-level gating.
//
// A single component manages a Deployment and a ConfigMap. The Deployment uses
// version-gated and boolean-gated mutations. The ConfigMap is gated at the
// resource level so it is created only when metrics are enabled.
package main

import (
	"context"
	"fmt"
	"os"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/app"
	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/metrics"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
)

func main() {
	scheme := runtime.NewScheme()
	mustAddToScheme(scheme, app.AddToScheme)
	mustAddToScheme(scheme, appsv1.AddToScheme)
	mustAddToScheme(scheme, corev1.AddToScheme)

	fakeClient := sharedapp.NewFakeClient(scheme, []sharedapp.RESTMapperEntry{
		{GVK: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Scope: meta.RESTScopeNamespace},
		{GVK: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, Scope: meta.RESTScopeNamespace},
		{GVK: sharedapp.GroupVersion.WithKind("ExampleApp"), Scope: meta.RESTScopeNamespace},
	})

	owner := &app.ExampleApp{
		Spec: app.ExampleAppSpec{
			Version:       "1.9.0",
			EnableTracing: true,
			EnableMetrics: true,
		},
	}
	owner.Name = "my-app"
	owner.Namespace = "default"

	ctx := context.Background()
	if err := fakeClient.Create(ctx, owner); err != nil {
		exit("failed to create owner: %v", err)
	}

	gauge := ocm.NewOperatorConditionsGauge("example")
	controller := &app.Controller{
		Client:                fakeClient,
		Scheme:                scheme,
		EventRecorder:         events.NewFakeRecorder(100),
		Metrics:               metrics.NewRecorder("example", gauge, metrics.NewCollectors()),
		NewDeploymentResource: resources.NewDeploymentResource,
		NewConfigMapResource:  resources.NewConfigMapResource,
	}

	steps := []app.ExampleAppSpec{
		// 1. v1: legacy container layout ("server", single port), tracing on.
		{Version: "1.9.0", EnableTracing: true, EnableMetrics: true},
		// 2. Enable debug logging on v1: LOG_LEVEL=debug targets "app" (baseline
		//    name) and carries through the legacy rename to "server".
		{Version: "1.9.0", EnableTracing: true, EnableMetrics: true, EnableDebugLogging: true},
		// 3. Upgrade to v2: container switches to "app" with health port added.
		{Version: "2.0.0", EnableTracing: true, EnableMetrics: true, EnableDebugLogging: true},
		// 4. Disable debug logging and tracing.
		{Version: "2.0.0", EnableTracing: false, EnableMetrics: true},
		// 5. Disable metrics: entire ConfigMap deleted via resource gating.
		{Version: "2.0.0", EnableTracing: false, EnableMetrics: false},
		// 6. Suspend the application.
		{Version: "2.0.0", EnableTracing: false, EnableMetrics: false, Suspended: true},
	}

	for i, spec := range steps {
		fmt.Printf("\n--- Step %d: Version=%s DebugLogging=%v Tracing=%v Metrics=%v Suspended=%v ---\n",
			i+1, spec.Version, spec.EnableDebugLogging, spec.EnableTracing, spec.EnableMetrics, spec.Suspended)

		owner.Spec = spec
		if err := fakeClient.Update(ctx, owner); err != nil {
			exit("failed to update owner: %v", err)
		}

		if err := controller.Reconcile(ctx, owner); err != nil {
			exit("reconciliation failed: %v", err)
		}

		for _, c := range owner.Status.Conditions {
			fmt.Printf("  Condition: %s  Status: %s  Reason: %s\n", c.Type, c.Status, c.Reason)
		}
	}

	fmt.Println("\nDone.")
}

func mustAddToScheme(scheme *runtime.Scheme, fn func(*runtime.Scheme) error) {
	if err := fn(scheme); err != nil {
		exit("failed to add to scheme: %v", err)
	}
}

func exit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
