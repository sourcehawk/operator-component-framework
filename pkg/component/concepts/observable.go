package concepts

import "sigs.k8s.io/controller-runtime/pkg/client"

// ObservationRecorder defines the contract for resources whose internal state must be
// updated with what was just fetched from the cluster, before any subsequent inspection
// (most notably data extraction) takes place.
//
// Read-only resources are constructed with a base object that typically carries only
// enough identifying metadata for the framework to fetch the live object. The fetched
// object must then be recorded back onto the resource so that capabilities such as
// [DataExtractable] observe the cluster state rather than the inert base.
//
// Implementations should accept any object compatible with the resource's underlying
// type and return an error when the supplied object is not assignable to that type.
type ObservationRecorder interface {
	// RecordObservation stores the supplied object as the resource's most recently
	// observed cluster state. Subsequent calls to capabilities that inspect the
	// resource's state (such as ExtractData) operate against this object.
	RecordObservation(observed client.Object) error
}
