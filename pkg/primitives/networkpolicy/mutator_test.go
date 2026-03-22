package networkpolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
)

func newTestNP() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-np",
			Namespace: "default",
		},
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", np.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditNetworkPolicySpec ---

func TestMutator_EditNetworkPolicySpec_SetPodSelector(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)
	m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
		e.SetPodSelector(metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "web"},
		})
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, map[string]string{"app": "web"}, np.Spec.PodSelector.MatchLabels)
}

func TestMutator_EditNetworkPolicySpec_Nil(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)
	m.EditNetworkPolicySpec(nil)
	assert.NoError(t, m.Apply())
}

func TestMutator_EditNetworkPolicySpec_IngressRules(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)

	port80 := intstr.FromInt32(80)
	tcp := corev1.ProtocolTCP

	m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
		e.EnsureIngressRule(networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port80}},
		})
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Len(t, np.Spec.Ingress, 1)
}

func TestMutator_EditNetworkPolicySpec_EgressRules(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)

	port443 := intstr.FromInt32(443)
	tcp := corev1.ProtocolTCP

	m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
		e.EnsureEgressRule(networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port443}},
		})
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Len(t, np.Spec.Egress, 1)
}

func TestMutator_EditNetworkPolicySpec_ReplaceIngressAtomically(t *testing.T) {
	port80 := intstr.FromInt32(80)
	port8080 := intstr.FromInt32(8080)
	tcp := corev1.ProtocolTCP

	np := newTestNP()
	np.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
		{Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port80}}},
	}

	m := NewMutator(np)
	m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
		e.RemoveIngressRules()
		e.EnsureIngressRule(networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port8080}},
		})
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, np.Spec.Ingress, 1)
	assert.Equal(t, &port8080, np.Spec.Ingress[0].Ports[0].Port)
}

func TestMutator_EditNetworkPolicySpec_SetPolicyTypes(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)
	m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
		e.SetPolicyTypes([]networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		})
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Len(t, np.Spec.PolicyTypes, 2)
}

func TestMutator_EditNetworkPolicySpec_Raw(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)
	m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
		raw := e.Raw()
		raw.PodSelector = metav1.LabelSelector{
			MatchLabels: map[string]string{"role": "db"},
		}
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, map[string]string{"role": "db"}, np.Spec.PodSelector.MatchLabels)
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)

	port80 := intstr.FromInt32(80)
	tcp := corev1.ProtocolTCP

	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
		e.EnsureIngressRule(networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port80}},
		})
		return nil
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", np.Labels["order"])
	assert.Len(t, np.Spec.Ingress, 1)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)

	port80 := intstr.FromInt32(80)
	port443 := intstr.FromInt32(443)
	tcp := corev1.ProtocolTCP

	m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
		e.EnsureIngressRule(networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port80}},
		})
		return nil
	})
	m.beginFeature()
	m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
		e.EnsureEgressRule(networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port443}},
		})
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Len(t, np.Spec.Ingress, 1)
	assert.Len(t, np.Spec.Egress, 1)
}

// --- Error propagation ---

func TestMutator_MetadataEditError(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		return errors.New("metadata error")
	})
	assert.Error(t, m.Apply())
}

func TestMutator_SpecEditError(t *testing.T) {
	np := newTestNP()
	m := NewMutator(np)
	m.EditNetworkPolicySpec(func(_ *editors.NetworkPolicySpecEditor) error {
		return errors.New("spec error")
	})
	assert.Error(t, m.Apply())
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
