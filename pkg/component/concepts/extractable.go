package concepts

// DataExtractable defines the contract for resources that need to expose internal data
// after they have been created, updated, or fetched from the cluster.
//
// Implement this interface when a resource contains information (like generated credentials,
// endpoint URLs, or status fields) that needs to be pulled back into the operator's
// memory for use by other components or for updating the parent CRD's status.
//
// Data extraction is intended to be an observational/read-only operation on the resource.
// It is triggered automatically during reconciliation after all resources have been
// synchronized with the cluster but before the final component condition is calculated.
type DataExtractable interface {
	// ExtractData performs the data extraction from the resource's underlying Kubernetes object.
	// The implementation should store the extracted data in its own fields or shared state
	// where it can be accessed by the caller.
	ExtractData() error
}
