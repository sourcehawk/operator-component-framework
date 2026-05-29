package concepts

import "sigs.k8s.io/controller-runtime/pkg/client"

// Previewable is implemented by resources that can render their post-mutation
// desired state as a client.Object without contacting the cluster.
//
// All built-in primitives satisfy this through generic.BaseResource. The
// component uses it to assemble a cluster-free preview of every managed resource
// it would apply (see the component package's Component.Preview).
type Previewable interface {
	// Preview renders the resource's desired state with all feature mutations
	// applied, leaving the resource's internal state untouched. Suspension
	// mutations are not applied; the preview reflects content state only.
	Preview() (client.Object, error)
}
