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

	assert.True(t, res.DeleteOnSuspend())
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
