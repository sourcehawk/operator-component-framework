// Package features provides sample mutations for the PVC primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pvc"
	"k8s.io/apimachinery/pkg/api/resource"
)

// VersionLabelMutation sets the app.kubernetes.io/version label on the PVC.
// It is always enabled.
func VersionLabelMutation(version string) pvc.Mutation {
	return pvc.Mutation{
		Name:    "version-label",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *pvc.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// LargeStorageMutation expands the PVC storage request to 50Gi.
// It is enabled when needsLargeStorage is true.
func LargeStorageMutation(version string, needsLargeStorage bool) pvc.Mutation {
	return pvc.Mutation{
		Name:    "large-storage",
		Feature: feature.NewResourceFeature(version, nil).When(needsLargeStorage),
		Mutate: func(m *pvc.Mutator) error {
			m.SetStorageRequest(resource.MustParse("50Gi"))
			return nil
		},
	}
}

// StorageAnnotationMutation adds a storage-class hint annotation to the PVC.
// It is always enabled.
func StorageAnnotationMutation(version string, storageClass string) pvc.Mutation {
	return pvc.Mutation{
		Name:    "storage-annotation",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *pvc.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureAnnotation("storage/class-hint", storageClass)
				return nil
			})
			return nil
		},
	}
}
