// Package scope provides utilities for determining Kubernetes resource scope
// compatibility, particularly for owner reference eligibility.
package scope

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// CanSetOwnerReference reports whether the owner can set an OwnerReference on the
// owned object. It returns false when the owned resource is cluster-scoped and the
// owner is namespace-scoped, which the Kubernetes API server rejects.
//
// Returns an error if the provided RESTMapper is nil.
func CanSetOwnerReference(
	owner, owned client.Object,
	scheme *runtime.Scheme,
	mapper meta.RESTMapper,
) (bool, error) {
	if mapper == nil {
		return false, fmt.Errorf("RESTMapper must not be nil")
	}

	ownedGVK, err := apiutil.GVKForObject(owned, scheme)
	if err != nil {
		return false, err
	}
	ownedMapping, err := mapper.RESTMapping(ownedGVK.GroupKind(), ownedGVK.Version)
	if err != nil {
		return false, err
	}

	// Namespace-scoped owned resources can always receive owner references.
	if ownedMapping.Scope.Name() != meta.RESTScopeNameRoot {
		return true, nil
	}

	// Owned is cluster-scoped — check if the owner is also cluster-scoped.
	ownerGVK, err := apiutil.GVKForObject(owner, scheme)
	if err != nil {
		return false, err
	}
	ownerMapping, err := mapper.RESTMapping(ownerGVK.GroupKind(), ownerGVK.Version)
	if err != nil {
		return false, err
	}

	// Only a cluster-scoped owner can own a cluster-scoped resource.
	return ownerMapping.Scope.Name() == meta.RESTScopeNameRoot, nil
}
