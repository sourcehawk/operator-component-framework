package generic

import "sigs.k8s.io/controller-runtime/pkg/client"

// FieldApplicationFlavor defines a function signature for applying "flavors" to a resource.
// A flavor typically preserves certain fields from the current (live) object after the
// baseline field application has occurred.
type FieldApplicationFlavor[T client.Object] func(applied, current, desired T) error
