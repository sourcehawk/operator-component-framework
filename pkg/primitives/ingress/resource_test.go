package ingress

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func newValidIngress() *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "test-ns",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ptr.To("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: ptr.To(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "web-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidIngress()).Build()
	require.NoError(t, err)
	assert.Equal(t, "networking.k8s.io/v1/Ingress/test-ns/test-ingress", res.Identity())
}

func TestResource_Object(t *testing.T) {
	ing := newValidIngress()
	res, err := NewBuilder(ing).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*networkingv1.Ingress)
	require.True(t, ok)
	assert.Equal(t, ing.Name, got.Name)
	assert.Equal(t, ing.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-ingress", ing.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidIngress()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*networkingv1.Ingress)
	assert.Equal(t, ptr.To("nginx"), got.Spec.IngressClassName)
	require.Len(t, got.Spec.Rules, 1)
	assert.Equal(t, "example.com", got.Spec.Rules[0].Host)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidIngress()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-tls",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
					e.EnsureTLS(networkingv1.IngressTLS{
						Hosts:      []string{"example.com"},
						SecretName: "tls-cert",
					})
					return nil
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*networkingv1.Ingress)
	assert.Equal(t, ptr.To("nginx"), got.Spec.IngressClassName)
	require.Len(t, got.Spec.TLS, 1)
	assert.Equal(t, "tls-cert", got.Spec.TLS[0].SecretName)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidIngress()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
					e.SetIngressClassName("traefik")
					return nil
				})
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
					e.SetIngressClassName("haproxy")
					return nil
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	// Last mutation wins.
	got := obj.(*networkingv1.Ingress)
	assert.Equal(t, ptr.To("haproxy"), got.Spec.IngressClassName)
}

func TestDefaultOperationalStatusHandler_Operational(t *testing.T) {
	ing := newValidIngress()
	ing.Status = networkingv1.IngressStatus{
		LoadBalancer: networkingv1.IngressLoadBalancerStatus{
			Ingress: []networkingv1.IngressLoadBalancerIngress{
				{IP: "10.0.0.1"},
			},
		},
	}

	status, err := DefaultOperationalStatusHandler(concepts.ConvergingOperationUpdated, ing)
	require.NoError(t, err)
	assert.Equal(t, concepts.OperationalStatusOperational, status.Status)
}

func TestDefaultOperationalStatusHandler_OperationalWithHostname(t *testing.T) {
	ing := newValidIngress()
	ing.Status = networkingv1.IngressStatus{
		LoadBalancer: networkingv1.IngressLoadBalancerStatus{
			Ingress: []networkingv1.IngressLoadBalancerIngress{
				{Hostname: "lb.example.com"},
			},
		},
	}

	status, err := DefaultOperationalStatusHandler(concepts.ConvergingOperationUpdated, ing)
	require.NoError(t, err)
	assert.Equal(t, concepts.OperationalStatusOperational, status.Status)
}

func TestResource_ConvergingStatus_Pending(t *testing.T) {
	res, err := NewBuilder(newValidIngress()).Build()
	require.NoError(t, err)

	status, err := res.ConvergingStatus(concepts.ConvergingOperationCreated)
	require.NoError(t, err)
	assert.Equal(t, concepts.OperationalStatusPending, status.Status)
}

func TestResource_DeleteOnSuspend(t *testing.T) {
	res, err := NewBuilder(newValidIngress()).Build()
	require.NoError(t, err)

	assert.False(t, res.DeleteOnSuspend())
}

func TestResource_Suspend(t *testing.T) {
	res, err := NewBuilder(newValidIngress()).Build()
	require.NoError(t, err)

	require.NoError(t, res.Suspend())
}

func TestResource_SuspensionStatus(t *testing.T) {
	res, err := NewBuilder(newValidIngress()).Build()
	require.NoError(t, err)

	status, err := res.SuspensionStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
}

func TestResource_ExtractData(t *testing.T) {
	ing := newValidIngress()

	var extracted string
	res, err := NewBuilder(ing).
		WithDataExtractor(func(i networkingv1.Ingress) error {
			extracted = *i.Spec.IngressClassName
			return nil
		}).
		Build()
	require.NoError(t, err)

	require.NoError(t, res.ExtractData())
	assert.Equal(t, "nginx", extracted)
}

func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidIngress()).
		WithDataExtractor(func(_ networkingv1.Ingress) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}
