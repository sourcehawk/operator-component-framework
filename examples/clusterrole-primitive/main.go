// Package main is the entry point for the clusterrole primitive example.
package main

import (
	"context"
	"fmt"
	"os"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/sourcehawk/operator-component-framework/examples/clusterrole-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/clusterrole-primitive/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	rbacv1 "k8s.io/api/rbac/v1"
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
	if err := rbacv1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add rbac/v1 to scheme: %v\n", err)
		os.Exit(1)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&sharedapp.ExampleApp{}).
		Build()

	// 2. Create an example Owner object.
	// NOTE: This example uses a namespaced owner for simplicity with the fake client.
	// The framework detects the scope mismatch between a namespaced owner and
	// a cluster-scoped dependent (ClusterRole) and skips setting the controller
	// reference (logging a message), so reconciliation still proceeds but without
	// garbage collection or owner-based adoption for the ClusterRole. If you want
	// owner references and GC/adoption for ClusterRoles in production, use a
	// cluster-scoped owner CRD.
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{
			Version:       "1.2.3",
			EnableTracing: true,
			EnableMetrics: true,
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
		NewClusterRoleResource: resources.NewClusterRoleResource,
	}

	// 4. Run reconciliation with multiple spec versions to demonstrate how
	//    feature-gated mutations compose RBAC rules from independent features.
	specs := []sharedapp.ExampleAppSpec{
		{
			Version:       "1.2.3",
			EnableTracing: true,
			EnableMetrics: true,
		},
		{
			Version:       "1.2.4", // Version upgrade
			EnableTracing: true,
			EnableMetrics: true,
		},
		{
			Version:       "1.2.4",
			EnableTracing: false, // Disable secret access
			EnableMetrics: true,
		},
		{
			Version:       "1.2.4",
			EnableTracing: false,
			EnableMetrics: false, // Disable deployment access too
		},
	}

	ctx := context.Background()

	for i, spec := range specs {
		fmt.Printf("\n--- Step %d: Version=%s, SecretAccess=%v, DeploymentAccess=%v ---\n",
			i+1, spec.Version, spec.EnableTracing, spec.EnableMetrics)

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
