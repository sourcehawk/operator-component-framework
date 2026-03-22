package rolebinding

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	rbacv1 "k8s.io/api/rbac/v1"
)

// FieldApplicationFlavor defines a function signature for applying flavors to a
// RoleBinding resource. A flavor is called after the baseline field applicator has
// run and can be used to preserve or merge fields from the live cluster object.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*rbacv1.RoleBinding]

// PreserveCurrentLabels ensures that any labels present on the current live
// RoleBinding but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *rbacv1.RoleBinding) error {
	return flavors.PreserveCurrentLabels[*rbacv1.RoleBinding]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live RoleBinding but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *rbacv1.RoleBinding) error {
	return flavors.PreserveCurrentAnnotations[*rbacv1.RoleBinding]()(applied, current, desired)
}
