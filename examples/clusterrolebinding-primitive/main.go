// Package main is the entry point for the clusterrolebinding primitive example.
package main

import (
	"fmt"
	"os"

	"github.com/sourcehawk/operator-component-framework/examples/clusterrolebinding-primitive/features"
	"github.com/sourcehawk/operator-component-framework/examples/clusterrolebinding-primitive/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	// 1. Create an example Owner object.
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{
			Version:       "1.2.3",
			EnableMetrics: true,
		},
	}
	owner.Name = "my-example-app"
	owner.Namespace = "default"

	// 2. Demonstrate building and mutating a ClusterRoleBinding through
	//    multiple spec variations. Each step builds a fresh resource and
	//    applies mutations to a simulated current object.
	specs := []sharedapp.ExampleAppSpec{
		{
			Version:       "1.2.3",
			EnableMetrics: true,
		},
		{
			Version:       "1.2.4", // Version upgrade
			EnableMetrics: true,
		},
		{
			Version:       "1.2.4",
			EnableMetrics: false, // Disable metrics — monitoring subject removed
		},
	}

	for i, spec := range specs {
		fmt.Printf("\n--- Step %d: Version=%s, Metrics=%v ---\n",
			i+1, spec.Version, spec.EnableMetrics)

		owner.Spec = spec

		// Build the resource from the factory.
		resource, err := resources.NewClusterRoleBindingResource(owner)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to build resource: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Identity: %s\n", resource.Identity())

		// Simulate a current cluster object.
		current := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:            owner.Name + "-binding",
				ResourceVersion: "12345",
				Labels:          map[string]string{"external-controller": "managed"},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     owner.Name + "-role",
			},
		}

		// Apply mutations to the current object.
		if err := resource.Mutate(current); err != nil {
			fmt.Fprintf(os.Stderr, "mutation failed: %v\n", err)
			os.Exit(1)
		}

		// Print the result.
		fmt.Printf("Labels: %v\n", current.Labels)
		fmt.Printf("RoleRef: %s/%s\n", current.RoleRef.Kind, current.RoleRef.Name)
		fmt.Printf("Subjects (%d):\n", len(current.Subjects))
		for _, s := range current.Subjects {
			fmt.Printf("  - %s %s/%s\n", s.Kind, s.Namespace, s.Name)
		}

		// Extract data.
		if err := resource.ExtractData(); err != nil {
			fmt.Fprintf(os.Stderr, "data extraction failed: %v\n", err)
			os.Exit(1)
		}
	}

	// 3. Demonstrate the version label mutation independently.
	fmt.Println("\n--- Standalone mutation demo ---")
	mutation := features.VersionLabelMutation("2.0.0")
	fmt.Printf("Mutation name: %s\n", mutation.Name)

	fmt.Println("\nExample completed successfully!")
}
