// Package main demonstrates managing a custom resource using the unstructured
// static builder.
//
// When a CRD has no typed primitive in the framework (e.g., a third-party
// CertificateRequest), the unstructured builder lets you manage it with the
// same mutation and extraction patterns as any built-in primitive.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/sourcehawk/operator-component-framework/examples/custom-resource/app"
	"github.com/sourcehawk/operator-component-framework/examples/custom-resource/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/meta"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
)

func main() {
	scheme := runtime.NewScheme()
	mustAddToScheme(scheme, app.AddToScheme)

	// Register the unstructured CertificateRequest type so the fake client
	// can round-trip it.
	scheme.AddKnownTypeWithName(
		resources.CertificateRequestGVK,
		&uns.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   resources.CertificateRequestGVK.Group,
			Version: resources.CertificateRequestGVK.Version,
			Kind:    resources.CertificateRequestGVK.Kind + "List",
		},
		&uns.UnstructuredList{},
	)

	fakeClient := sharedapp.NewFakeClient(scheme, []sharedapp.RESTMapperEntry{
		{GVK: resources.CertificateRequestGVK, Scope: meta.RESTScopeNamespace},
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

	// Condition metrics and resource-level apply metrics are both recorded
	// through one component.MetricsRecorder. Register the collectors once per
	// process; a real operator would use controller-runtime's registry:
	//
	//	ctrlmetrics.Registry.MustRegister(gauge, collectors)
	gauge := ocm.NewOperatorConditionsGauge("example")
	collectors := metrics.NewCollectors()
	registry := prometheus.NewRegistry()
	registry.MustRegister(gauge, collectors)

	controller := &app.Controller{
		Client:                 fakeClient,
		Scheme:                 scheme,
		EventRecorder:          events.NewFakeRecorder(100),
		Metrics:                metrics.NewRecorder("example", gauge, collectors),
		NewCertificateResource: resources.NewCertificateResource,
	}

	// Step 1: Create the CertificateRequest.
	fmt.Println("--- Step 1: Create certificate ---")
	if err := controller.Reconcile(ctx, owner); err != nil {
		exit("reconciliation failed: %v", err)
	}
	printConditions(owner)

	// Step 2: Reconcile again (steady state).
	fmt.Println("\n--- Step 2: Steady-state reconciliation ---")
	if err := controller.Reconcile(ctx, owner); err != nil {
		exit("reconciliation failed: %v", err)
	}
	printConditions(owner)

	// The apply counters record what each reconcile did to the resource: one
	// created, then none. A resource whose "updated" count keeps climbing in
	// steady state is being rewritten on every pass for no reason.
	fmt.Println("\n--- Resource apply metrics ---")
	printApplyMetrics(registry)

	fmt.Println("\nDone.")
}

func printConditions(owner *app.ExampleApp) {
	for _, c := range owner.Status.Conditions {
		fmt.Printf("  Condition: %s  Status: %s  Reason: %s\n", c.Type, c.Status, c.Reason)
	}
}

func printApplyMetrics(registry *prometheus.Registry) {
	families, err := registry.Gather()
	if err != nil {
		exit("failed to gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "ocf_resource_apply_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make([]string, 0, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels = append(labels, fmt.Sprintf("%s=%q", label.GetName(), label.GetValue()))
			}
			fmt.Printf(
				"  %s{%s} %g\n",
				family.GetName(), strings.Join(labels, ","), metric.GetCounter().GetValue(),
			)
		}
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
