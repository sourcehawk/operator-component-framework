// Package main demonstrates declared data extraction and guard-based resource
// ordering.
//
// A single component manages a ConfigMap and a Secret. The ConfigMap declares
// an extraction into a shared data cell, and the Secret declares a data guard
// on that cell, blocking reconciliation until the ConfigMap has produced it.
package main

import (
	"context"
	"fmt"
	"os"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/app"
	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/metrics"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
)

func main() {
	scheme := runtime.NewScheme()
	mustAddToScheme(scheme, app.AddToScheme)
	mustAddToScheme(scheme, corev1.AddToScheme)

	fakeClient := sharedapp.NewFakeClient(scheme, []sharedapp.RESTMapperEntry{
		{GVK: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, Scope: meta.RESTScopeNamespace},
		{GVK: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, Scope: meta.RESTScopeNamespace},
		{GVK: sharedapp.GroupVersion.WithKind("ExampleApp"), Scope: meta.RESTScopeNamespace},
	})

	owner := &app.ExampleApp{
		Spec: app.ExampleAppSpec{Version: "1.0.0"},
	}
	owner.Name = "my-app"
	owner.Namespace = "default"

	ctx := context.Background()
	if err := fakeClient.Create(ctx, owner); err != nil {
		exit("failed to create owner: %v", err)
	}

	gauge := ocm.NewOperatorConditionsGauge("example")
	controller := &app.Controller{
		Client:               fakeClient,
		Scheme:               scheme,
		EventRecorder:        events.NewFakeRecorder(100),
		Metrics:              metrics.NewRecorder("example", gauge, metrics.NewCollectors()),
		NewConfigMapResource: resources.NewConfigMapResource,
		NewSecretResource:    resources.NewSecretResource,
	}

	comp, _, err := controller.BuildComponent(owner)
	if err != nil {
		exit("failed to build component: %v", err)
	}
	fmt.Println("--- Declared data topology ---")
	for _, edge := range comp.DataTopology() {
		fmt.Printf("  data %q: producers=%v guarded=%v optional=%v\n", edge.Data, edge.Producers, edge.Guarded, edge.Optional)
	}

	// Step 1: Normal reconciliation. The ConfigMap is created first, its
	// declared extraction captures db-host, and the Secret's data guard
	// unblocks.
	fmt.Println("\n--- Step 1: Normal reconciliation ---")
	if err := controller.Reconcile(ctx, owner); err != nil {
		exit("reconciliation failed: %v", err)
	}
	printConditions(owner)

	// Step 2: Reconcile again to show steady-state behavior.
	fmt.Println("\n--- Step 2: Steady-state reconciliation ---")
	if err := controller.Reconcile(ctx, owner); err != nil {
		exit("reconciliation failed: %v", err)
	}
	printConditions(owner)

	fmt.Println("\nDone.")
}

func printConditions(owner *app.ExampleApp) {
	for _, c := range owner.Status.Conditions {
		fmt.Printf("  Condition: %s  Status: %s  Reason: %s\n", c.Type, c.Status, c.Reason)
	}
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
