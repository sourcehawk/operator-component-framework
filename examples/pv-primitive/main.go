// Package main is the entry point for the pv primitive example.
package main

import (
	"fmt"
	"os"

	"github.com/sourcehawk/operator-component-framework/examples/pv-primitive/features"
	"github.com/sourcehawk/operator-component-framework/examples/pv-primitive/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func main() {
	// 1. Create an example Owner object.
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{
			Version:       "1.0.0",
			EnableTracing: false,
			EnableMetrics: false,
		},
	}
	owner.Name = "my-storage-app"
	owner.Namespace = "default"

	// 2. Run through multiple spec variations to demonstrate how mutations compose.
	//    EnableTracing gates the retain-policy mutation; EnableMetrics gates mount options.
	specs := []sharedapp.ExampleAppSpec{
		{
			Version:       "1.0.0",
			EnableTracing: false,
			EnableMetrics: false,
		},
		{
			Version:       "2.0.0", // Version upgrade
			EnableTracing: false,
			EnableMetrics: false,
		},
		{
			Version:       "2.0.0",
			EnableTracing: true, // Enable retain policy
			EnableMetrics: false,
		},
		{
			Version:       "2.0.0",
			EnableTracing: true,
			EnableMetrics: true, // Enable mount options
		},
	}

	for i, spec := range specs {
		fmt.Printf("\n--- Step %d: Version=%s, RetainPolicy=%v, MountOpts=%v ---\n",
			i+1, spec.Version, spec.EnableTracing, spec.EnableMetrics)

		owner.Spec = spec

		// Build the PV resource for this spec.
		pvResource, err := resources.NewPVResource(owner)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to build PV resource: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Identity: %s\n", pvResource.Identity())

		// Simulate a current PV from the cluster (what CreateOrUpdate would provide).
		current := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:            owner.Name + "-data",
				ResourceVersion: "12345", // non-empty to simulate existing object
				Annotations: map[string]string{
					"external-controller/managed": "true", // should be preserved by flavor
				},
			},
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("50Gi"),
				},
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{Path: "/mnt/old-data"},
				},
			},
		}

		// Apply mutations to the current object.
		if err := pvResource.Mutate(current); err != nil {
			fmt.Fprintf(os.Stderr, "mutation failed: %v\n", err)
			os.Exit(1)
		}

		// Print the result.
		y, err := yaml.Marshal(current)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal PV: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Resulting PV:\n%s", string(y))

		// Show that immutable fields were preserved from current.
		fmt.Printf("Preserved volume source path: %s (original: /mnt/old-data)\n",
			current.Spec.PersistentVolumeSource.HostPath.Path)
	}

	// 3. Demonstrate the operational status handler.
	fmt.Println("\n--- Operational Status Examples ---")
	for _, phase := range []corev1.PersistentVolumePhase{
		corev1.VolumeAvailable, corev1.VolumeBound, corev1.VolumePending,
		corev1.VolumeReleased, corev1.VolumeFailed,
	} {
		status, _ := features.ExampleOperationalStatus(phase)
		fmt.Printf("Phase %-10s → Status: %s, Reason: %s\n", phase, status.Status, status.Reason)
	}

	fmt.Println("\nExample completed successfully!")
}
