package concepts

// DataExtractable is the runtime hook through which the component triggers a
// resource's declared data extractions (see ExtractInto on the builders).
// Extraction runs immediately after each resource is applied or fetched during
// reconciliation, so data extracted from one resource is available to
// subsequent resources' guards and mutations within the same cycle, and always
// before the final component condition is calculated.
//
// All built-in primitives satisfy this through generic.BaseResource. User code
// does not call ExtractData; declare extractions on the builder instead.
type DataExtractable interface {
	// ExtractData performs the data extraction from the resource's underlying Kubernetes object.
	// The implementation should store the extracted data in its own fields or shared state
	// where it can be accessed by the caller.
	ExtractData() error
}
