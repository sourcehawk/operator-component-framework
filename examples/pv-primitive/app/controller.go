// Package app provides a sample controller using the pv primitive.
//
// PersistentVolumes are cluster-scoped resources. In a real operator, the owner
// would typically be a cluster-scoped CRD (e.g. a ClusterStorageConfig), or the
// PV resource would be created and managed without using a component create
// pipeline that unconditionally sets controller references, to avoid the
// controller-runtime restriction that cluster-scoped resources cannot have
// namespace-scoped owners.
//
// This example demonstrates the PV primitive's builder, mutation, and status APIs
// without full component reconciliation to keep the focus on the primitive itself.
package app
