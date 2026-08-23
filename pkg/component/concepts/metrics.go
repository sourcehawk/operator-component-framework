package concepts

// MetricsIdentifiable is implemented by resources that carry a stable,
// low-cardinality identifier for resource-level metrics.
//
// The framework reads the identifier when it applies the resource and uses it
// as the value of the `resource` label. A resource that does not implement the
// interface, or that returns an empty string, is labelled with its lowercased
// kind instead.
type MetricsIdentifiable interface {
	// MetricsIdentifier returns the resource's metrics identifier, or an empty
	// string to accept the framework's default.
	MetricsIdentifier() string
}
