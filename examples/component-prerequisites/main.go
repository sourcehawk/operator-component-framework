// Package main demonstrates component-level prerequisites with DependsOn.
//
// Two components share one owner. The "infra" component manages a ConfigMap and
// reports InfraReady. The "app" component manages a Deployment and uses
// DependsOn("InfraReady") to wait for the infra component before proceeding.
package main

import (
	"context"
	"fmt"
	"os"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/app"
	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
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
		Client:        fakeClient,
		Scheme:        scheme,
		EventRecorder: events.NewFakeRecorder(100),
		Metrics: &ocm.ConditionMetricRecorder{
			Controller:              "example",
			OperatorConditionsGauge: gauge,
		},
		NewConfigMapResource:  resources.NewConfigMapResource,
		NewDeploymentResource: resources.NewDeploymentResource,
	}

	// Step 1: Full reconciliation. Infra runs first and sets InfraReady=True,
	// then the app component's prerequisite passes and it proceeds.
	fmt.Println("--- Step 1: Full reconciliation ---")
	if err := controller.Reconcile(ctx, owner); err != nil {
		exit("reconciliation failed: %v", err)
	}
	printConditions(owner)

	// Step 2: Version upgrade. The prerequisite was already satisfied, so it is
	// never re-evaluated. Both components reconcile normally.
	fmt.Println("\n--- Step 2: Version upgrade ---")
	owner.Spec.Version = "1.1.0"
	if err := fakeClient.Update(ctx, owner); err != nil {
		exit("failed to update owner: %v", err)
	}
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
