// Package main demonstrates the assembly and reconciliation of a Component-based application.
package main

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/sourcehawk/operator-component-framework/examples/component-architecture-basics/exampleapp"
)

// mockMetricsRecorder is a simple no-op implementation of component.Recorder for our example.
type mockMetricsRecorder struct{}

func (m *mockMetricsRecorder) RecordConditionFor(
	_ string, _ ocm.ObjectLike,
	_, _, _ string, _ time.Time,
	_ ...string,
) {
	// No-op for the example
}

// main demonstrates the assembly and reconciliation of a Component-based application.
// It shows how different versions of an application trigger different
// Feature Mutations, resulting in different Kubernetes resource states.
func main() {
	// Set up a logger and inject it into the context.
	// This ensures that logs from within the framework and our data extractors
	// are visible in the output.
	logger := zap.New(zap.UseDevMode(true))
	log.SetLogger(logger)
	ctx := log.IntoContext(context.Background(), logger)

	// Initialize a fake Kubernetes client and scheme for our example.
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	scheme.AddKnownTypes(schema.GroupVersion{Group: "apps.example.io", Version: "v1"}, &exampleapp.ExamplePlatform{})

	controller := &exampleapp.ExampleController{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&exampleapp.ExamplePlatform{}).Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Metrics:  &mockMetricsRecorder{},
	}

	// For our fake client to work with status updates, we should seed the owner objects.
	_ = controller.Client.Create(ctx, &exampleapp.ExamplePlatform{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps.example.io/v1", Kind: "ExamplePlatform"},
		ObjectMeta: metav1.ObjectMeta{Name: "example-legacy", Namespace: "default", UID: "uid-790"},
		Spec:       exampleapp.ExamplePlatformSpec{Version: "7.9.0"},
	})
	_ = controller.Client.Create(ctx, &exampleapp.ExamplePlatform{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps.example.io/v1", Kind: "ExamplePlatform"},
		ObjectMeta: metav1.ObjectMeta{Name: "example-modern", Namespace: "default", UID: "uid-810"},
		Spec:       exampleapp.ExamplePlatformSpec{Version: "8.1.0"},
	})
	_ = controller.Client.Create(ctx, &exampleapp.ExamplePlatform{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps.example.io/v1", Kind: "ExamplePlatform"},
		ObjectMeta: metav1.ObjectMeta{Name: "example-latest", Namespace: "default", UID: "uid-900"},
		Spec:       exampleapp.ExamplePlatformSpec{Version: "9.0.0"},
	})

	// 1. Reconcile an older version (Legacy compatibility enabled, Tracing disabled, Cleanup enabled)
	fmt.Println("=== Scenario 1: Reconciling Legacy Version 7.9.0 ===")
	owner790 := &exampleapp.ExamplePlatform{}
	if err := controller.Client.Get(ctx, client.ObjectKey{Name: "example-legacy", Namespace: "default"}, owner790); err != nil {
		fmt.Printf("Error getting owner: %v\n", err)
	}
	if err := controller.Reconcile(ctx, owner790); err != nil {
		fmt.Printf("Error reconciling Legacy version: %v\n", err)
	}
	printOwnerConditions(owner790)

	// 2. Reconcile a modern version (Legacy compatibility enabled, Tracing enabled)
	fmt.Println("\n=== Scenario 2: Reconciling Modern Version 8.1.0 ===")
	owner810 := &exampleapp.ExamplePlatform{}
	if err := controller.Client.Get(ctx, client.ObjectKey{Name: "example-modern", Namespace: "default"}, owner810); err != nil {
		fmt.Printf("Error getting owner: %v\n", err)
	}
	if err := controller.Reconcile(ctx, owner810); err != nil {
		fmt.Printf("Error reconciling Modern version: %v\n", err)
	}
	printOwnerConditions(owner810)

	// 3. Reconcile latest version (Legacy compatibility disabled, Tracing enabled)
	fmt.Println("\n=== Scenario 3: Reconciling Latest Version 9.0.0 ===")
	owner900 := &exampleapp.ExamplePlatform{}
	if err := controller.Client.Get(ctx, client.ObjectKey{Name: "example-latest", Namespace: "default"}, owner900); err != nil {
		fmt.Printf("Error getting owner: %v\n", err)
	}
	if err := controller.Reconcile(ctx, owner900); err != nil {
		fmt.Printf("Error reconciling Latest version: %v\n", err)
	}
	printOwnerConditions(owner900)

	// 4. Test Suspension
	fmt.Println("\n=== Scenario 4: Suspending the Component ===")
	// Get the latest version from the client to have the correct ResourceVersion.
	if err := controller.Client.Get(ctx, client.ObjectKey{Name: "example-latest", Namespace: "default"}, owner900); err != nil {
		fmt.Printf("Error getting owner: %v\n", err)
	}
	owner900.Spec.Suspended = true
	// Update the object in the client so that the controller can see the suspension state.
	if err := controller.Client.Update(ctx, owner900); err != nil {
		fmt.Printf("Error updating owner for suspension: %v\n", err)
	}
	if err := controller.Reconcile(ctx, owner900); err != nil {
		fmt.Printf("Error reconciling Suspended version: %v\n", err)
	}
	printOwnerConditions(owner900)
}

// printOwnerConditions is a helper for displaying the status conditions aggregated by the Component.
func printOwnerConditions(owner *exampleapp.ExamplePlatform) {
	fmt.Printf("Owner: %s (Version: %s, Suspended: %t)\n", owner.Name, owner.Spec.Version, owner.Spec.Suspended)
	for _, c := range owner.Status.Conditions {
		fmt.Printf("  Condition %s: %s (Reason: %s, Message: %s)\n", c.Type, c.Status, c.Reason, c.Message)
	}
}
