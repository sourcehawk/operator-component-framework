package generic

import "sigs.k8s.io/controller-runtime/pkg/client"

// StaticResource is a generic internal resource implementation for Kubernetes objects
// that are treated as static desired-state resources, such as ConfigMaps and Secrets.
//
// It supports:
//   - default or custom baseline field application
//   - post-baseline field application flavors
//   - feature mutations
//   - data extraction after reconciliation
//
// Unlike workload, task, and integration resources, it does not support convergence
// status, grace status, or suspension behavior.
type StaticResource[T client.Object, M MutatorApplier] struct {
	BaseResource[T, M]
}
