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
	m.BeginFeature()
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
	m.BeginFeature()
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditRules ---

func TestMutator_EditRules(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.BeginFeature()
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
	m.BeginFeature()
	m.EditRules(nil)
	assert.NoError(t, m.Apply())
}

// --- AddRule ---

func TestMutator_AddRule(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.BeginFeature()
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
	m.BeginFeature()
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
	m.BeginFeature()
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
	m.BeginFeature()
	m.SetAggregationRule(nil)
	require.NoError(t, m.Apply())
	assert.Nil(t, cr.AggregationRule)
}

func TestMutator_SetAggregationRule_LastWins(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.BeginFeature()
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

func TestMutator_SetAggregationRule_Immutable(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.BeginFeature()
	aggRule := &rbacv1.AggregationRule{
		ClusterRoleSelectors: []metav1.LabelSelector{
			{MatchLabels: map[string]string{"aggregate": "true"}},
		},
	}
	m.SetAggregationRule(aggRule)

	// Mutate the original after registration — should not affect the mutator.
	aggRule.ClusterRoleSelectors[0].MatchLabels["aggregate"] = "mutated"

	require.NoError(t, m.Apply())
	require.NotNil(t, cr.AggregationRule)
	assert.Equal(t, "true", cr.AggregationRule.ClusterRoleSelectors[0].MatchLabels["aggregate"],
		"mutating the original after SetAggregationRule should not affect the applied result")
}

// --- Execution order ---

func TestMutator_MixedOperationTypes(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.BeginFeature()
	// Register both metadata and rules mutations to verify they are all applied.
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("mixed", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", cr.Labels["mixed"])
	require.Len(t, cr.Rules, 1)
	assert.Equal(t, "pods", cr.Rules[0].Resources[0])
}

func TestMutator_MultipleMutations(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.BeginFeature()
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
	})
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"list"},
	})
	require.NoError(t, m.Apply())

	assert.Len(t, cr.Rules, 2)
	assert.Equal(t, "pods", cr.Rules[0].Resources[0])
	assert.Equal(t, "services", cr.Rules[1].Resources[0])
}

// --- Feature boundaries ---

func TestMutator_MultipleFeatures(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)

	// Feature A: add a rule and a label
	m.BeginFeature()
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("feature-a", "true")
		return nil
	})

	// Feature B: add another rule and a label
	m.BeginFeature()
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"list"},
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("feature-b", "true")
		return nil
	})

	require.NoError(t, m.Apply())

	// Both features' edits applied in order
	assert.Equal(t, "true", cr.Labels["feature-a"])
	assert.Equal(t, "true", cr.Labels["feature-b"])
	require.Len(t, cr.Rules, 2)
	assert.Equal(t, "pods", cr.Rules[0].Resources[0])
	assert.Equal(t, "deployments", cr.Rules[1].Resources[0])
}

func TestMutator_FeatureMixedEdits(t *testing.T) {
	// Within a single feature, both metadata and rules edits are applied.
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.BeginFeature()

	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("feature", "complete")
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Equal(t, "complete", cr.Labels["feature"])
	require.Len(t, cr.Rules, 1)
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}

// --- Constructor and feature plan invariants ---

func TestNewMutator_InitializesNoPlan(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)

	assert.Empty(t, m.plans, "NewMutator must not create any plans")
	assert.Nil(t, m.active, "active plan must not be set")
}

func TestBeginFeature_AddsExactlyOnePlan(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)

	m.BeginFeature()
	require.Len(t, m.plans, 1, "BeginFeature must add exactly one plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the new plan")

	m.BeginFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestBeginFeature_IsolatesFeaturePlans(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)

	// Record a mutation in the first feature plan
	m.BeginFeature()
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
	})

	// Start a new feature and record a different mutation
	m.BeginFeature()
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"list"},
	})

	assert.Len(t, m.plans[0].rulesEdits, 1, "first plan should have one rules edit")
	assert.Len(t, m.plans[1].rulesEdits, 1, "second plan should have one rules edit")
}

func TestMutator_PanicsWithoutBeginFeature(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)

	assert.PanicsWithValue(t,
		"clusterrole.Mutator: BeginFeature must be called before registering mutations",
		func() { m.EditObjectMetadata(func(*editors.ObjectMetaEditor) error { return nil }) },
		"EditObjectMetadata should panic without BeginFeature",
	)
	assert.PanicsWithValue(t,
		"clusterrole.Mutator: BeginFeature must be called before registering mutations",
		func() { m.EditRules(func(*editors.PolicyRulesEditor) error { return nil }) },
		"EditRules should panic without BeginFeature",
	)
	assert.PanicsWithValue(t,
		"clusterrole.Mutator: BeginFeature must be called before registering mutations",
		func() { m.SetAggregationRule(nil) },
		"SetAggregationRule should panic without BeginFeature",
	)
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	cr := newTestCR(nil)
	m := NewMutator(cr)
	m.BeginFeature()
	m.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
	})

	require.NoError(t, m.Apply())
	assert.Len(t, m.plans, 1)
}
