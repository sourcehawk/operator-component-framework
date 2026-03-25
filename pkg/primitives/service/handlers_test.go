package service

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestDefaultOperationalStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		svc        *corev1.Service
		wantStatus concepts.OperationalStatus
		wantReason string
	}{
		{
			name: "ClusterIP is immediately operational",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "Service is operational",
		},
		{
			name: "NodePort is immediately operational",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeNodePort,
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "Service is operational",
		},
		{
			name: "ExternalName is immediately operational",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeExternalName,
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "Service is operational",
		},
		{
			name: "service with default type (empty spec) is immediately operational",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "Service is operational",
		},
		{
			name: "LoadBalancer pending (no ingress)",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
				Status: corev1.ServiceStatus{},
			},
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "Awaiting load balancer IP/hostname assignment",
		},
		{
			name: "LoadBalancer pending (empty ingress slice)",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{},
					},
				},
			},
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "Awaiting load balancer IP/hostname assignment",
		},
		{
			name: "LoadBalancer pending (ingress entry without IP or hostname)",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{},
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "Awaiting load balancer IP/hostname assignment",
		},
		{
			name: "LoadBalancer operational (IP assigned)",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{IP: "203.0.113.10"},
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "Load balancer IP/hostname assigned",
		},
		{
			name: "LoadBalancer operational (hostname assigned)",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{Hostname: "abc123.elb.amazonaws.com"},
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "Load balancer IP/hostname assigned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultOperationalStatusHandler(concepts.ConvergingOperationNone, tt.svc)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	svc := &corev1.Service{}
	assert.False(t, DefaultDeleteOnSuspendHandler(svc))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	svc := &corev1.Service{}
	mutator := NewMutator(svc)
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
	// No-op, so service should be unchanged
	require.NoError(t, mutator.Apply())
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	svc := &corev1.Service{}
	got, err := DefaultSuspensionStatusHandler(svc)
	require.NoError(t, err)
	assert.Equal(t, concepts.SuspensionStatusSuspended, got.Status)
	assert.Equal(t, "Service unaffected by suspension", got.Reason)
}
