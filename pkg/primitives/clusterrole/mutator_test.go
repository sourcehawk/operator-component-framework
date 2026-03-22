package clusterrole

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestCR(rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cr",
		},
		Rules: rules,
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", cr.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditRules ---

func TestMutator_EditRules(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.EditRules(func(e *editors.PolicyRulesEditor) error {
		e.AddRule(rbacv1.PolicyRule{
			APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
		})
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, cr.Rules, 1)
	assert.Equal(t, "pods", cr.Rules[0].Resources[0])
}

func TestMutator_EditRules_Nil(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.EditRules(nil)
	assert.NoError(t, m.Apply())
}

// --- AddRule ---

func TestMutator_AddRule(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list"},
	})
	require.NoError(t, m.Apply())
	require.Len(t, cr.Rules, 1)
	assert.Equal(t, "deployments", cr.Rules[0].Resources[0])
}

func TestMutator_AddRule_Appends(t *testing.T) {
	existing := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	}
	cr := newTestCR(existing)
	m := NewMutator(cr)
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"list"},
	})
	require.NoError(t, m.Apply())
	assert.Len(t, cr.Rules, 2)
}

// --- SetAggregationRule ---

func TestMutator_SetAggregationRule(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	aggRule := &rbacv1.AggregationRule{
		ClusterRoleSelectors: []metav1.LabelSelector{
			{MatchLabels: map[string]string{"aggregate": "true"}},
		},
	}
	m.SetAggregationRule(aggRule)
	require.NoError(t, m.Apply())
	require.NotNil(t, cr.AggregationRule)
	assert.Len(t, cr.AggregationRule.ClusterRoleSelectors, 1)
	assert.Equal(t, "true", cr.AggregationRule.ClusterRoleSelectors[0].MatchLabels["aggregate"])
}

func TestMutator_SetAggregationRule_Nil(t *testing.T) {
	cr := newTestCR(nil)
	cr.AggregationRule = &rbacv1.AggregationRule{
		ClusterRoleSelectors: []metav1.LabelSelector{
			{MatchLabels: map[string]string{"old": "rule"}},
		},
	}
	m := NewMutator(cr)
	m.SetAggregationRule(nil)
	require.NoError(t, m.Apply())
	assert.Nil(t, cr.AggregationRule)
}

func TestMutator_SetAggregationRule_LastWins(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.SetAggregationRule(&rbacv1.AggregationRule{
		ClusterRoleSelectors: []metav1.LabelSelector{
			{MatchLabels: map[string]string{"first": "true"}},
		},
	})
	m.SetAggregationRule(&rbacv1.AggregationRule{
		ClusterRoleSelectors: []metav1.LabelSelector{
			{MatchLabels: map[string]string{"second": "true"}},
		},
	})
	require.NoError(t, m.Apply())
	require.NotNil(t, cr.AggregationRule)
	assert.Equal(t, "true", cr.AggregationRule.ClusterRoleSelectors[0].MatchLabels["second"])
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", cr.Labels["order"])
	require.Len(t, cr.Rules, 1)
	assert.Equal(t, "pods", cr.Rules[0].Resources[0])
}

func TestMutator_MultipleFeatures(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
	})
	m.beginFeature()
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"list"},
	})
	require.NoError(t, m.Apply())

	assert.Len(t, cr.Rules, 2)
	assert.Equal(t, "pods", cr.Rules[0].Resources[0])
	assert.Equal(t, "services", cr.Rules[1].Resources[0])
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
