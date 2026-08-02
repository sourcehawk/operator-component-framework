package concepts

// DataConsumption records one declared read of a data cell by a resource.
type DataConsumption struct {
	// Cell is the cell being read.
	Cell DataCell
	// Optional reports the read mode: false means the resource blocks until
	// the cell is set (WithDataGuard); true means the resource proceeds and
	// reads opportunistically (WithOptionalData).
	Optional bool
}

// DataProducer is implemented by resources that declare data extractions.
// The component builder uses it to validate that every consumed cell has a
// producer registered strictly earlier, and the component uses it to know
// which cells to clear at the start of each reconcile.
type DataProducer interface {
	// ProducedData returns the cells this resource extracts into, deduplicated,
	// in declaration order.
	ProducedData() []DataCell
}

// DataConsumer is implemented by resources that declare data reads, either
// blocking (WithDataGuard) or optional (WithOptionalData).
type DataConsumer interface {
	// ConsumedData returns the declared reads in declaration order.
	ConsumedData() []DataConsumption
}

// DataEdge describes the declared flow of one data cell through a component:
// which resources write it and which resources read it.
type DataEdge struct {
	// Data is the cell name.
	Data string
	// Producers lists the resource identities declaring a write, in
	// registration order.
	Producers []string
	// Guarded lists the resource identities blocking on the cell, in
	// registration order.
	Guarded []string
	// Optional lists the resource identities optionally reading the cell, in
	// registration order.
	Optional []string
}

// DataInspector surfaces, read-only, the declared data topology of a built
// component. It is the data-flow counterpart of MutationInspector: an inert
// capability that nothing in the reconcile path calls, so importing it costs
// nothing at runtime.
type DataInspector interface {
	// DataTopology returns one edge per declared cell, in first-producer
	// registration order.
	DataTopology() []DataEdge
}
