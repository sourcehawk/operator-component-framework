package ingress

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

func TestDefaultOperationalStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		op         concepts.ConvergingOperation
		ingress    *networkingv1.Ingress
		wantStatus concepts.OperationalStatus
		wantReason string
	}{
		{
			name: "pending with no load balancer ingress",
			op:   concepts.ConvergingOperationCreated,
			ingress: &networkingv1.Ingress{
				Status: networkingv1.IngressStatus{},
			},
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "Awaiting load balancer address assignment",
		},
		{
			name: "pending with empty load balancer ingress slice",
			op:   concepts.ConvergingOperationUpdated,
			ingress: &networkingv1.Ingress{
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{
						Ingress: []networkingv1.IngressLoadBalancerIngress{},
					},
				},
			},
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "Awaiting load balancer address assignment",
		},
		{
			name: "pending with non-empty slice but empty entries",
			op:   concepts.ConvergingOperationCreated,
			ingress: &networkingv1.Ingress{
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{
						Ingress: []networkingv1.IngressLoadBalancerIngress{
							{},
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "Awaiting load balancer address assignment",
		},
		{
			name: "operational with IP assigned",
			op:   concepts.ConvergingOperationCreated,
			ingress: &networkingv1.Ingress{
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{
						Ingress: []networkingv1.IngressLoadBalancerIngress{
							{IP: "10.0.0.1"},
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "Ingress has been assigned an address",
		},
		{
			name: "operational with hostname assigned",
			op:   concepts.ConvergingOperationNone,
			ingress: &networkingv1.Ingress{
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{
						Ingress: []networkingv1.IngressLoadBalancerIngress{
							{Hostname: "lb.example.com"},
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "Ingress has been assigned an address",
		},
		{
			name: "operational with multiple entries",
			op:   concepts.ConvergingOperationUpdated,
			ingress: &networkingv1.Ingress{
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{
						Ingress: []networkingv1.IngressLoadBalancerIngress{
							{IP: "10.0.0.1"},
							{Hostname: "lb.example.com"},
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "Ingress has been assigned an address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultOperationalStatusHandler(tt.op, tt.ingress)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	ing := &networkingv1.Ingress{}
	assert.False(t, DefaultDeleteOnSuspendHandler(ing))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	ing := &networkingv1.Ingress{
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "example.com"},
			},
		},
	}
	mutator := NewMutator(ing)
	mutator.BeginFeature()
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
	// No-op: rules should remain unchanged.
	assert.Len(t, ing.Spec.Rules, 1)
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	t.Run("always reports suspended", func(t *testing.T) {
		ing := &networkingv1.Ingress{}
		got, err := DefaultSuspensionStatusHandler(ing)
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, got.Status)
		assert.Equal(t, "Ingress suspended (backend unavailable)", got.Reason)
	})

	t.Run("suspended even with load balancer assigned", func(t *testing.T) {
		ing := &networkingv1.Ingress{
			Status: networkingv1.IngressStatus{
				LoadBalancer: networkingv1.IngressLoadBalancerStatus{
					Ingress: []networkingv1.IngressLoadBalancerIngress{
						{IP: "10.0.0.1"},
					},
				},
			},
		}
		got, err := DefaultSuspensionStatusHandler(ing)
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, got.Status)
	})
}

// Ensure the handler ignores the ConvergingOperation parameter (tested with all values).
func TestDefaultOperationalStatusHandler_IgnoresOperation(t *testing.T) {
	ing := &networkingv1.Ingress{
		Status: networkingv1.IngressStatus{
			LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{
					{IP: "10.0.0.1"},
				},
			},
		},
	}

	ops := []concepts.ConvergingOperation{
		concepts.ConvergingOperationCreated,
		concepts.ConvergingOperationUpdated,
		concepts.ConvergingOperationNone,
	}

	for _, op := range ops {
		got, err := DefaultOperationalStatusHandler(op, ing)
		require.NoError(t, err)
		assert.Equal(t, concepts.OperationalStatusOperational, got.Status)
	}
}

// Verify the handler works with the core/v1 LoadBalancerIngress type embedded
// in the networking/v1 IngressLoadBalancerIngress. The Ports field is a
// networking/v1 extension.
func TestDefaultOperationalStatusHandler_WithPorts(t *testing.T) {
	ing := &networkingv1.Ingress{
		Status: networkingv1.IngressStatus{
			LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{
					{
						IP: "10.0.0.1",
						Ports: []networkingv1.IngressPortStatus{
							{
								Port:     80,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}

	got, err := DefaultOperationalStatusHandler(concepts.ConvergingOperationCreated, ing)
	require.NoError(t, err)
	assert.Equal(t, concepts.OperationalStatusOperational, got.Status)
}
