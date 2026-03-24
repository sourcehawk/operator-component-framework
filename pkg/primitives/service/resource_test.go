package service

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newValidService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
			},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidService()).Build()
	require.NoError(t, err)
	assert.Equal(t, "v1/Service/test-ns/test-svc", res.Identity())
}

func TestResource_Object(t *testing.T) {
	svc := newValidService()
	res, err := NewBuilder(svc).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*corev1.Service)
	require.True(t, ok)
	assert.Equal(t, svc.Name, got.Name)
	assert.Equal(t, svc.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-svc", svc.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidService()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	current := &corev1.Service{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, "test", current.Spec.Selector["app"])
	assert.Equal(t, int32(80), current.Spec.Ports[0].Port)
}

func TestResource_Mutate_PreservesClusterIP(t *testing.T) {
	desired := newValidService()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	current := &corev1.Service{
		Spec: corev1.ServiceSpec{
			ClusterIP:  "10.0.0.1",
			ClusterIPs: []string{"10.0.0.1"},
		},
	}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, "10.0.0.1", current.Spec.ClusterIP)
	assert.Equal(t, []string{"10.0.0.1"}, current.Spec.ClusterIPs)
	assert.Equal(t, "test", current.Spec.Selector["app"])
}

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-svc",
			Namespace:       "test-ns",
			ResourceVersion: "12345",
			UID:             types.UID("abc-def"),
			Generation:      3,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Pod", Name: "other-owner", UID: "other-uid"},
			},
			Finalizers: []string{"finalizer.example.com"},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:  "10.0.0.1",
			ClusterIPs: []string{"10.0.0.1"},
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports:    []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec and labels are applied
	assert.Equal(t, "test", current.Labels["app"])
	assert.Equal(t, "test", current.Spec.Selector["app"])
	assert.Equal(t, int32(80), current.Spec.Ports[0].Port)

	// Server-managed fields are preserved
	assert.Equal(t, "12345", current.ResourceVersion)
	assert.Equal(t, types.UID("abc-def"), current.UID)
	assert.Equal(t, int64(3), current.Generation)

	// Shared-controller fields are preserved
	assert.Len(t, current.OwnerReferences, 1)
	assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)

	// Immutable ClusterIP fields are preserved
	assert.Equal(t, "10.0.0.1", current.Spec.ClusterIP)
	assert.Equal(t, []string{"10.0.0.1"}, current.Spec.ClusterIPs)
}

func TestDefaultFieldApplicator_PreservesStatus(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeLoadBalancer,
			ClusterIP: "10.0.0.1",
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{IP: "203.0.113.1", Hostname: "lb.example.com"},
				},
			},
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"app": "test"},
			Ports:    []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec is applied
	assert.Equal(t, "test", current.Spec.Selector["app"])

	// Status is preserved (not zeroed by the deep copy of desired)
	require.Len(t, current.Status.LoadBalancer.Ingress, 1)
	assert.Equal(t, "203.0.113.1", current.Status.LoadBalancer.Ingress[0].IP)
	assert.Equal(t, "lb.example.com", current.Status.LoadBalancer.Ingress[0].Hostname)
}

func TestDefaultFieldApplicator_PreservesNodePorts(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeNodePort,
			ClusterIP: "10.0.0.1",
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP},
				{Name: "https", Port: 443, NodePort: 30443, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: map[string]string{"app": "test"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Auto-allocated nodePorts are preserved when desired nodePort is 0
	require.Len(t, current.Spec.Ports, 2)
	assert.Equal(t, int32(30080), current.Spec.Ports[0].NodePort)
	assert.Equal(t, int32(30443), current.Spec.Ports[1].NodePort)
}

func TestDefaultFieldApplicator_DoesNotPreserveNodePortsForClusterIP(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeNodePort,
			ClusterIP: "10.0.0.1",
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": "test"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// NodePort should NOT be preserved when switching to ClusterIP
	assert.Equal(t, int32(0), current.Spec.Ports[0].NodePort)
	assert.Equal(t, corev1.ServiceTypeClusterIP, current.Spec.Type)
}

func TestDefaultFieldApplicator_DoesNotPreserveNodePortsForEmptyType(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeNodePort,
			ClusterIP: "10.0.0.1",
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			// Empty type defaults to ClusterIP
			Selector: map[string]string{"app": "test"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// NodePort should NOT be preserved when desired type is empty (defaults to ClusterIP)
	assert.Equal(t, int32(0), current.Spec.Ports[0].NodePort)
}

func TestDefaultFieldApplicator_PreservesNodePortsForLoadBalancer(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeLoadBalancer,
			ClusterIP: "10.0.0.1",
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"app": "test"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// NodePort SHOULD be preserved for LoadBalancer type
	assert.Equal(t, int32(30080), current.Spec.Ports[0].NodePort)
}

func TestDefaultFieldApplicator_PreservesIPFamilyFields(t *testing.T) {
	singleStack := corev1.IPFamilyPolicySingleStack
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:      "10.0.0.1",
			IPFamilies:     []corev1.IPFamily{corev1.IPv4Protocol},
			IPFamilyPolicy: &singleStack,
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports:    []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Server-defaulted IP family fields are preserved
	assert.Equal(t, []corev1.IPFamily{corev1.IPv4Protocol}, current.Spec.IPFamilies)
	require.NotNil(t, current.Spec.IPFamilyPolicy)
	assert.Equal(t, corev1.IPFamilyPolicySingleStack, *current.Spec.IPFamilyPolicy)
}

func TestDefaultFieldApplicator_ExplicitIPFamilyOverridesPreservation(t *testing.T) {
	singleStack := corev1.IPFamilyPolicySingleStack
	preferDualStack := corev1.IPFamilyPolicyPreferDualStack
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:      "10.0.0.1",
			IPFamilies:     []corev1.IPFamily{corev1.IPv4Protocol},
			IPFamilyPolicy: &singleStack,
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Selector:       map[string]string{"app": "test"},
			Ports:          []corev1.ServicePort{{Name: "http", Port: 80}},
			IPFamilies:     []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			IPFamilyPolicy: &preferDualStack,
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Explicitly set IP family fields in desired take precedence
	assert.Equal(t, []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, current.Spec.IPFamilies)
	require.NotNil(t, current.Spec.IPFamilyPolicy)
	assert.Equal(t, corev1.IPFamilyPolicyPreferDualStack, *current.Spec.IPFamilyPolicy)
}

func TestDefaultFieldApplicator_PreservesHealthCheckNodePort(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeLoadBalancer,
			ClusterIP:             "10.0.0.1",
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
			HealthCheckNodePort:   31234,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 30080},
			},
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeLoadBalancer,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
			Selector:              map[string]string{"app": "test"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Server-allocated HealthCheckNodePort is preserved
	assert.Equal(t, int32(31234), current.Spec.HealthCheckNodePort)
}

func TestDefaultFieldApplicator_ExplicitHealthCheckNodePortOverridesPreservation(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeLoadBalancer,
			ClusterIP:             "10.0.0.1",
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
			HealthCheckNodePort:   31234,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 30080},
			},
		},
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeLoadBalancer,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
			HealthCheckNodePort:   32000,
			Selector:              map[string]string{"app": "test"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Explicitly set HealthCheckNodePort in desired takes precedence
	assert.Equal(t, int32(32000), current.Spec.HealthCheckNodePort)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidService()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-port",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
					e.EnsurePort(corev1.ServicePort{Name: "metrics", Port: 9090})
					return nil
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	current := &corev1.Service{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, int32(80), current.Spec.Ports[0].Port)
	require.Len(t, current.Spec.Ports, 2)
	assert.Equal(t, "metrics", current.Spec.Ports[1].Name)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidService()

	var callOrder []string
	observedFirstBeforeSecond := false

	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "first-mutation",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				callOrder = append(callOrder, "first")
				m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
					e.EnsurePort(corev1.ServicePort{Name: "first", Port: 1000})
					return nil
				})
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "second-mutation",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(_ *Mutator) error {
				if len(callOrder) == 1 && callOrder[0] == "first" {
					observedFirstBeforeSecond = true
				}
				callOrder = append(callOrder, "second")
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	current := &corev1.Service{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, []string{"first", "second"}, callOrder)
	assert.True(t, observedFirstBeforeSecond, "second mutation function should execute after the first")
}

func TestResource_Mutate_CustomFieldApplicator(t *testing.T) {
	desired := newValidService()

	applicatorCalled := false
	res, err := NewBuilder(desired).
		WithCustomFieldApplicator(func(current, d *corev1.Service) error {
			applicatorCalled = true
			current.Spec.Ports = d.DeepCopy().Spec.Ports
			return nil
		}).
		Build()
	require.NoError(t, err)

	current := &corev1.Service{
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"external": "preserved"},
		},
	}
	require.NoError(t, res.Mutate(current))

	assert.True(t, applicatorCalled)
	assert.Equal(t, int32(80), current.Spec.Ports[0].Port)
	assert.Equal(t, "preserved", current.Spec.Selector["external"])
}

func TestResource_Mutate_CustomFieldApplicator_Error(t *testing.T) {
	res, err := NewBuilder(newValidService()).
		WithCustomFieldApplicator(func(_, _ *corev1.Service) error {
			return errors.New("applicator error")
		}).
		Build()
	require.NoError(t, err)

	err = res.Mutate(&corev1.Service{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applicator error")
}

func TestResource_ConvergingStatus(t *testing.T) {
	svc := newValidService()
	svc.Spec.Type = corev1.ServiceTypeLoadBalancer

	res, err := NewBuilder(svc).Build()
	require.NoError(t, err)

	status, err := res.ConvergingStatus(concepts.ConvergingOperationNone)
	require.NoError(t, err)
	assert.Equal(t, concepts.OperationalStatusPending, status.Status)
}

func TestResource_Suspend(t *testing.T) {
	res, err := NewBuilder(newValidService()).Build()
	require.NoError(t, err)

	require.NoError(t, res.Suspend())
}

func TestResource_DeleteOnSuspend(t *testing.T) {
	res, err := NewBuilder(newValidService()).Build()
	require.NoError(t, err)

	assert.False(t, res.DeleteOnSuspend())
}

func TestResource_SuspensionStatus(t *testing.T) {
	res, err := NewBuilder(newValidService()).Build()
	require.NoError(t, err)

	status, err := res.SuspensionStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
}

func TestResource_ExtractData(t *testing.T) {
	svc := newValidService()

	var extracted string
	res, err := NewBuilder(svc).
		WithDataExtractor(func(s corev1.Service) error {
			extracted = s.Spec.Selector["app"]
			return nil
		}).
		Build()
	require.NoError(t, err)

	require.NoError(t, res.ExtractData())
	assert.Equal(t, "test", extracted)
}

func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidService()).
		WithDataExtractor(func(_ corev1.Service) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}
