// Package main is the entry point for the statefulset primitive example.
package main

import (
	"context"
	"fmt"
	"os"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/sourcehawk/operator-component-framework/examples/statefulset-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/statefulset-primitive/resources"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func main() {
	// 1. Setup scheme and fake client for the example.
	scheme := runtime.NewScheme()
	if err := app.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add to scheme: %v\n", err)
		os.Exit(1)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add apps/v1 to scheme: %v\n", err)
		os.Exit(1)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&app.ExampleApp{}).
		Build()

	// 2. Create an example Owner object.
	owner := &app.ExampleApp{
		Spec: app.ExampleAppSpec{
			Version:       "1.2.3",
			EnableTracing: true,
			EnableMetrics: true,
			Suspended:     false,
		},
	}
	owner.Name = "my-example-app"
	owner.Namespace = "default"

	if err := fakeClient.Create(context.Background(), owner); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create owner: %v\n", err)
		os.Exit(1)
	}

	// 3. Initialize our controller.
	gauge := ocm.NewOperatorConditionsGauge("example")
	controller := &app.ExampleController{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(100),
		Metrics: &ocm.ConditionMetricRecorder{
			Controller:              "example-controller",
			OperatorConditionsGauge: gauge,
		},

		NewStatefulSetResource: resources.NewStatefulSetResource,
	}

	// 4. Run reconciliation with multiple spec versions.
	specs := []app.ExampleAppSpec{
		{
			Version:       "1.2.3",
			EnableTracing: true,
			EnableMetrics: true,
			Suspended:     false,
		},
		{
			Version:       "1.2.4", // Version upgrade
			EnableTracing: true,
			EnableMetrics: true,
			Suspended:     false,
		},
		{
			Version:       "1.2.4",
			EnableTracing: false, // Disable tracing
			EnableMetrics: true,
			Suspended:     false,
		},
		{
			Version:       "1.2.4",
			EnableTracing: false,
			EnableMetrics: true,
			Suspended:     true, // Suspend the app
		},
	}

	ctx := context.Background()

	for i, spec := range specs {
		fmt.Printf("\n--- Step %d: Applying Spec: Version=%s, Tracing=%v, Metrics=%v, Suspended=%v ---\n",
			i+1, spec.Version, spec.EnableTracing, spec.EnableMetrics, spec.Suspended)

		// Update owner spec
		owner.Spec = spec
		if err := fakeClient.Update(ctx, owner); err != nil {
			fmt.Fprintf(os.Stderr, "failed to update owner: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Running reconciliation...")
		if err := controller.Reconcile(ctx, owner); err != nil {
			fmt.Fprintf(os.Stderr, "reconciliation failed: %v\n", err)
			os.Exit(1)
		}

		// Inspect the owner conditions.
		for _, cond := range owner.Status.Conditions {
			fmt.Printf("Condition: %s, Status: %s, Reason: %s\n",
				cond.Type, cond.Status, cond.Reason)
		}
	}

	fmt.Println("\nReconciliation sequence completed successfully!")
}
