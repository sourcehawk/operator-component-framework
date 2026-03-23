// Package app provides a sample controller using the pv primitive.
//
// PersistentVolumes are cluster-scoped resources. In a real operator, the owner
// would typically be a cluster-scoped CRD (e.g. a ClusterStorageConfig). When a
// component create pipeline is used, it will only set controller references where
// the owner/owned scopes are compatible, and will skip ownerReferences for
// cluster-scoped resources that would otherwise have namespace-scoped owners. In
// those skipped cases, the cluster-scoped resource will not be garbage-collected
// with the namespaced owner.
//
// This example demonstrates the PV primitive's builder, mutation, and status APIs
// without full component reconciliation to keep the focus on the primitive itself.
package app
