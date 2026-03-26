package generic

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IntegrationResource is a generic internal resource implementation for Kubernetes
// integration objects such as Services, Ingresses, and Gateways.
//
// It provides shared behavior for:
//   - baseline field application
//   - feature mutations
//   - data extraction
//
// Concrete integration packages are expected to wrap this type and provide kind-specific
// identity and status logic.
type IntegrationResource[T client.Object, M FeatureMutator] struct {
	BaseResource[T, M]

	OperationalStatusHandler func(concepts.ConvergingOperation, T) (concepts.OperationalStatusWithReason, error)
}

// ConvergingStatus reports the integration's operational status using the configured handler.
func (r *IntegrationResource[T, M]) ConvergingStatus(
	op concepts.ConvergingOperation,
) (concepts.OperationalStatusWithReason, error) {
	if r.OperationalStatusHandler == nil {
		return concepts.OperationalStatusWithReason{}, fmt.Errorf("operational status handler is not configured")
	}
	return r.OperationalStatusHandler(op, r.DesiredObject)
}
