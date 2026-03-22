// Package main is the entry point for the PDB primitive example.
package main

import (
	"context"
	"fmt"
	"os"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/sourcehawk/operator-component-framework/examples/pdb-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/pdb-primitive/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	policyv1 "k8s.io/api/policy/v1"
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
	if err := policyv1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add policy/v1 to scheme: %v\n", err)
		os.Exit(1)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&sharedapp.ExampleApp{}).
		Build()

	// 2. Create an example Owner object.
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{
			Version:       "1.0.0",
			EnableMetrics: false,
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
		NewPDBResource: resources.NewPDBResource,
	}

	// 4. Run reconciliation with multiple spec versions to demonstrate how
	//    feature-gated mutations modify the PDB disruption policy.
	specs := []sharedapp.ExampleAppSpec{
		{
			Version:       "1.0.0",
			EnableMetrics: false,
		},
		{
			Version:       "1.1.0", // Version upgrade
			EnableMetrics: false,
		},
		{
			Version:       "1.1.0",
			EnableMetrics: true, // Enable metrics → stricter availability
		},
		{
			Version:       "1.1.0",
			EnableMetrics: false, // Disable metrics → back to default
		},
	}

	ctx := context.Background()

	for i, spec := range specs {
		fmt.Printf("\n--- Step %d: Version=%s, Metrics=%v ---\n",
			i+1, spec.Version, spec.EnableMetrics)

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
