// Package resources provides resource implementations for the PDB primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/pdb-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pdb"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NewPDBResource constructs a PDB primitive resource with all the features.
func NewPDBResource(owner *sharedapp.ExampleApp) (component.Resource, error) {
	// 1. Create the base PodDisruptionBudget object.
	minAvailable := intstr.FromString("50%")
	base := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-pdb",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": owner.Name,
				},
			},
		},
	}

	// 2. Initialize the PDB builder.
	builder := pdb.NewBuilder(base)

	// 3. Register mutations in dependency order.
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.StrictAvailabilityMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	// 4. Preserve labels added by external controllers.
	builder.WithFieldApplicationFlavor(pdb.PreserveCurrentLabels)

	// 5. Extract data from the reconciled PDB.
	builder.WithDataExtractor(func(p policyv1.PodDisruptionBudget) error {
		fmt.Printf("Reconciled PDB: %s\n", p.Name)
		if p.Spec.MinAvailable != nil {
			fmt.Printf("  MinAvailable: %s\n", p.Spec.MinAvailable.String())
		}
		if p.Spec.MaxUnavailable != nil {
			fmt.Printf("  MaxUnavailable: %s\n", p.Spec.MaxUnavailable.String())
		}
		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
