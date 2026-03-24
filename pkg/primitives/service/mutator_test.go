package service

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", svc.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

func TestMutator_EditObjectMetadata_Error(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		return errors.New("metadata error")
	})
	err := m.Apply()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata error")
}

// --- EditServiceSpec ---

func TestMutator_EditServiceSpec(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.SetType(corev1.ServiceTypeNodePort)
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, corev1.ServiceTypeNodePort, svc.Spec.Type)
}

func TestMutator_EditServiceSpec_Nil(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(nil)
	assert.NoError(t, m.Apply())
}

func TestMutator_EditServiceSpec_Error(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(_ *editors.ServiceSpecEditor) error {
		return errors.New("spec error")
	})
	err := m.Apply()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec error")
}

func TestMutator_EditServiceSpec_Ports(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
		e.EnsurePort(corev1.ServicePort{Name: "https", Port: 443})
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Len(t, svc.Spec.Ports, 2)
}

func TestMutator_EditServiceSpec_Selector(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsureSelector("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", svc.Spec.Selector["app"])
}

func TestMutator_EditServiceSpec_RawAccess(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.Raw().ExternalName = "external.example.com"
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "external.example.com", svc.Spec.ExternalName)
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before service spec edits.
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
		return nil
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", svc.Labels["order"])
	assert.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, "http", svc.Spec.Ports[0].Name)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
		return nil
	})
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "https", Port: 443})
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Len(t, svc.Spec.Ports, 2)
}

func TestMutator_MultipleFeatures_LaterSeesEarlier(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsureSelector("app", "myapp")
		return nil
	})
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		// Later feature should see the selector set by the earlier feature.
		e.EnsureSelector("env", "prod")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "myapp", svc.Spec.Selector["app"])
	assert.Equal(t, "prod", svc.Spec.Selector["env"])
}

// --- NodePort preservation across mutations ---

func TestMutator_Apply_PreservesNodePortsAfterEnsurePort(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 30080},
				{Name: "https", Port: 443, NodePort: 30443},
			},
		},
	}
	m := NewMutator(svc)
	m.BeginFeature()
	// EnsurePort replaces the entire ServicePort, which zeroes NodePort.
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, int32(30080), svc.Spec.Ports[0].NodePort, "auto-allocated NodePort should be restored")
	assert.Equal(t, int32(30443), svc.Spec.Ports[1].NodePort, "untouched port should keep its NodePort")
}

func TestMutator_Apply_PreservesNodePortsForLoadBalancer(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 31080},
			},
		},
	}
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, int32(31080), svc.Spec.Ports[0].NodePort)
}

func TestMutator_Apply_ExplicitNodePortNotOverridden(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 30080},
			},
		},
	}
	m := NewMutator(svc)
	m.BeginFeature()
	// Explicitly set a different NodePort — should not be reverted to the snapshot.
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80, NodePort: 32000})
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, int32(32000), svc.Spec.Ports[0].NodePort, "explicitly set NodePort should not be overridden")
}

func TestMutator_Apply_NoNodePortPreservationForClusterIP(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 30080},
			},
		},
	}
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, int32(0), svc.Spec.Ports[0].NodePort, "ClusterIP services should not preserve NodePorts")
}

// --- Constructor and feature plan invariants ---

func TestNewMutator_InitializesNoPlan(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)

	assert.Empty(t, m.plans, "NewMutator must not create any plans")
	assert.Nil(t, m.active, "active plan must not be set")
}

func TestBeginFeature_AddsExactlyOnePlan(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)

	m.BeginFeature()
	require.Len(t, m.plans, 1, "BeginFeature must add exactly one plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the new plan")

	m.BeginFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestBeginFeature_IsolatesFeaturePlans(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)

	// Record a mutation in the first feature plan
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
		return nil
	})

	// Start a new feature and record a different mutation
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "https", Port: 443})
		return nil
	})

	assert.Len(t, m.plans[0].serviceSpecEdits, 1, "first plan should have one spec edit")
	assert.Len(t, m.plans[1].serviceSpecEdits, 1, "second plan should have one spec edit")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	svc := newTestService()
	m := NewMutator(svc)
	m.BeginFeature()
	m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
		e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Len(t, svc.Spec.Ports, 1)
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
