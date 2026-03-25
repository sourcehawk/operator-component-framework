// Package main is the entry point for the service primitive example.
package main

import (
	"context"
	"fmt"
	"os"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/sourcehawk/operator-component-framework/examples/service-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/service-primitive/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func main() {
	// 1. Setup scheme and fake client.
	scheme := runtime.NewScheme()
	if err := sharedapp.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add to scheme: %v\n", err)
		os.Exit(1)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add core/v1 to scheme: %v\n", err)
		os.Exit(1)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&sharedapp.ExampleApp{}).
		Build()

	// 2. Create an example Owner object.
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{
			Version:       "1.2.3",
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

	// 3. Initialize the controller.
	gauge := ocm.NewOperatorConditionsGauge("example")
	controller := &app.ExampleController{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(100),
		Metrics: &ocm.ConditionMetricRecorder{
			Controller:              "example-controller",
			OperatorConditionsGauge: gauge,
		},
		NewServiceResource: resources.NewServiceResource,
	}

	// 4. Run reconciliation with multiple spec versions to demonstrate
	//    how mutations compose service configuration.
	specs := []sharedapp.ExampleAppSpec{
		{
			Version:       "1.2.3",
			EnableMetrics: true,
			Suspended:     false,
		},
		{
			Version:       "1.2.4", // Version upgrade
			EnableMetrics: true,
			Suspended:     false,
		},
		{
			Version:       "1.2.4",
			EnableMetrics: false, // Disable metrics
			Suspended:     false,
		},
		{
			Version:       "1.2.4",
			EnableMetrics: false,
			Suspended:     true, // Suspend the app
		},
	}

	ctx := context.Background()

	for i, spec := range specs {
		fmt.Printf("\n--- Step %d: Version=%s, Metrics=%v, Suspended=%v ---\n",
			i+1, spec.Version, spec.EnableMetrics, spec.Suspended)

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

		for _, cond := range owner.Status.Conditions {
			fmt.Printf("Condition: %s, Status: %s, Reason: %s\n",
				cond.Type, cond.Status, cond.Reason)
		}
	}

	fmt.Println("\nReconciliation sequence completed successfully!")
}
