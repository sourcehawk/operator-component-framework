package networkpolicy

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestResource_Identity(t *testing.T) {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-netpol",
			Namespace: "test-ns",
		},
	}
	res, err := NewBuilder(np).Build()
	require.NoError(t, err)

	assert.Equal(t, "networking.k8s.io/v1/NetworkPolicy/test-ns/test-netpol", res.Identity())
}

func TestResource_Object(t *testing.T) {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-netpol",
			Namespace: "test-ns",
		},
	}
	res, err := NewBuilder(np).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*networkingv1.NetworkPolicy)
	require.True(t, ok)
	assert.Equal(t, np.Name, got.Name)
	assert.Equal(t, np.Namespace, got.Namespace)

	// Ensure it's a deep copy
	got.Name = "changed"
	assert.Equal(t, "test-netpol", np.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	port := intstr.FromInt32(8080)
	tcp := corev1.ProtocolTCP

	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "test-mutation",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
					e.AppendIngressRule(networkingv1.NetworkPolicyIngressRule{
						Ports: []networkingv1.NetworkPolicyPort{
							{Protocol: &tcp, Port: &port},
						},
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

	got := obj.(*networkingv1.NetworkPolicy)
	assert.Equal(t, "test", got.Labels["app"])
	assert.Equal(t, "test", got.Spec.PodSelector.MatchLabels["app"])
	require.Len(t, got.Spec.Ingress, 1)
	require.Len(t, got.Spec.Ingress[0].Ports, 1)
	assert.Equal(t, int32(8080), got.Spec.Ingress[0].Ports[0].Port.IntVal)
}

func TestResource_Mutate_DisabledFeature(t *testing.T) {
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "disabled-mutation",
			Feature: feature.NewVersionGate("v1", nil).When(false),
			Mutate: func(m *Mutator) error {
				m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
					e.EnsureLabel("should-not", "appear")
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

	got := obj.(*networkingv1.NetworkPolicy)
	assert.NotContains(t, got.Labels, "should-not")
}

func TestResource_Mutate_NilFeatureIsUnconditional(t *testing.T) {
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name: "unconditional",
			Mutate: func(m *Mutator) error {
				m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
					e.EnsureLabel("always", "present")
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

	got := obj.(*networkingv1.NetworkPolicy)
	assert.Equal(t, "present", got.Labels["always"])
}

func TestResource_Mutate_MutationOrdering(t *testing.T) {
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	port80 := intstr.FromInt32(80)
	port443 := intstr.FromInt32(443)
	tcp := corev1.ProtocolTCP

	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name: "http",
			Mutate: func(m *Mutator) error {
				m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
					e.AppendIngressRule(networkingv1.NetworkPolicyIngressRule{
						Ports: []networkingv1.NetworkPolicyPort{
							{Protocol: &tcp, Port: &port80},
						},
					})
					return nil
				})
				return nil
			},
		}).
		WithMutation(Mutation{
			Name: "https",
			Mutate: func(m *Mutator) error {
				m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
					e.AppendIngressRule(networkingv1.NetworkPolicyIngressRule{
						Ports: []networkingv1.NetworkPolicyPort{
							{Protocol: &tcp, Port: &port443},
						},
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

	got := obj.(*networkingv1.NetworkPolicy)
	require.Len(t, got.Spec.Ingress, 2)
	assert.Equal(t, int32(80), got.Spec.Ingress[0].Ports[0].Port.IntVal)
	assert.Equal(t, int32(443), got.Spec.Ingress[1].Ports[0].Port.IntVal)
}

func TestResource_ExtractData(t *testing.T) {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	extractedSelector := ""
	res, err := NewBuilder(np).
		WithDataExtractor(func(np networkingv1.NetworkPolicy) error {
			extractedSelector = np.Spec.PodSelector.MatchLabels["app"]
			return nil
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.NoError(t, err)
	assert.Equal(t, "test", extractedSelector)
}
