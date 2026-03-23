package ingress

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestIngress() *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ing",
			Namespace: "default",
		},
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", ing.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

func TestMutator_EditObjectMetadata_Error(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		return errors.New("metadata error")
	})
	err := m.Apply()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata error")
}

// --- EditIngressSpec ---

func TestMutator_EditIngressSpec(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)
	m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
		e.SetIngressClassName("nginx")
		return nil
	})
	require.NoError(t, m.Apply())
	require.NotNil(t, ing.Spec.IngressClassName)
	assert.Equal(t, "nginx", *ing.Spec.IngressClassName)
}

func TestMutator_EditIngressSpec_Nil(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)
	m.EditIngressSpec(nil)
	assert.NoError(t, m.Apply())
}

func TestMutator_EditIngressSpec_Error(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)
	m.EditIngressSpec(func(_ *editors.IngressSpecEditor) error {
		return errors.New("spec error")
	})
	err := m.Apply()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec error")
}

func TestMutator_EditIngressSpec_EnsureRule(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)
	m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
		e.EnsureRule(networkingv1.IngressRule{Host: "example.com"})
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, ing.Spec.Rules, 1)
	assert.Equal(t, "example.com", ing.Spec.Rules[0].Host)
}

func TestMutator_EditIngressSpec_EnsureTLS(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)
	m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
		e.EnsureTLS(networkingv1.IngressTLS{
			Hosts:      []string{"example.com"},
			SecretName: "tls-secret",
		})
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, ing.Spec.TLS, 1)
	assert.Equal(t, "tls-secret", ing.Spec.TLS[0].SecretName)
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before spec edits.
	ing := newTestIngress()
	m := NewMutator(ing)

	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
		e.SetIngressClassName("nginx")
		return nil
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Equal(t, "tested", ing.Labels["order"])
	require.NotNil(t, ing.Spec.IngressClassName)
	assert.Equal(t, "nginx", *ing.Spec.IngressClassName)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)
	m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
		e.EnsureRule(networkingv1.IngressRule{Host: "feature1.com"})
		return nil
	})
	m.BeginFeature()
	m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
		e.EnsureRule(networkingv1.IngressRule{Host: "feature2.com"})
		return nil
	})
	require.NoError(t, m.Apply())

	require.Len(t, ing.Spec.Rules, 2)
	assert.Equal(t, "feature1.com", ing.Spec.Rules[0].Host)
	assert.Equal(t, "feature2.com", ing.Spec.Rules[1].Host)
}

func TestMutator_CombinedMutations(t *testing.T) {
	ing := newTestIngress()
	m := NewMutator(ing)

	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "web")
		e.EnsureAnnotation("description", "main ingress")
		return nil
	})
	m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
		e.SetIngressClassName("nginx")
		e.EnsureRule(networkingv1.IngressRule{Host: "example.com"})
		e.EnsureTLS(networkingv1.IngressTLS{
			Hosts:      []string{"example.com"},
			SecretName: "tls-cert",
		})
		return nil
	})

	require.NoError(t, m.Apply())

	assert.Equal(t, "web", ing.Labels["app"])
	assert.Equal(t, "main ingress", ing.Annotations["description"])
	require.NotNil(t, ing.Spec.IngressClassName)
	assert.Equal(t, "nginx", *ing.Spec.IngressClassName)
	require.Len(t, ing.Spec.Rules, 1)
	require.Len(t, ing.Spec.TLS, 1)
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
