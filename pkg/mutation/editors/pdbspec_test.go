package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestPodDisruptionBudgetSpecEditor_SetMinAvailable(t *testing.T) {
	t.Parallel()
	spec := &policyv1.PodDisruptionBudgetSpec{}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	e.SetMinAvailable(intstr.FromInt32(2))

	require.NotNil(t, spec.MinAvailable)
	assert.Equal(t, intstr.FromInt32(2), *spec.MinAvailable)
}

func TestPodDisruptionBudgetSpecEditor_SetMinAvailable_Percentage(t *testing.T) {
	t.Parallel()
	spec := &policyv1.PodDisruptionBudgetSpec{}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	e.SetMinAvailable(intstr.FromString("50%"))

	require.NotNil(t, spec.MinAvailable)
	assert.Equal(t, intstr.FromString("50%"), *spec.MinAvailable)
}

func TestPodDisruptionBudgetSpecEditor_SetMaxUnavailable(t *testing.T) {
	t.Parallel()
	spec := &policyv1.PodDisruptionBudgetSpec{}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	e.SetMaxUnavailable(intstr.FromInt32(1))

	require.NotNil(t, spec.MaxUnavailable)
	assert.Equal(t, intstr.FromInt32(1), *spec.MaxUnavailable)
}

func TestPodDisruptionBudgetSpecEditor_SetMaxUnavailable_Percentage(t *testing.T) {
	t.Parallel()
	spec := &policyv1.PodDisruptionBudgetSpec{}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	e.SetMaxUnavailable(intstr.FromString("25%"))

	require.NotNil(t, spec.MaxUnavailable)
	assert.Equal(t, intstr.FromString("25%"), *spec.MaxUnavailable)
}

func TestPodDisruptionBudgetSpecEditor_ClearMinAvailable(t *testing.T) {
	t.Parallel()
	val := intstr.FromInt32(2)
	spec := &policyv1.PodDisruptionBudgetSpec{MinAvailable: &val}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	e.ClearMinAvailable()

	assert.Nil(t, spec.MinAvailable)
}

func TestPodDisruptionBudgetSpecEditor_ClearMaxUnavailable(t *testing.T) {
	t.Parallel()
	val := intstr.FromInt32(1)
	spec := &policyv1.PodDisruptionBudgetSpec{MaxUnavailable: &val}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	e.ClearMaxUnavailable()

	assert.Nil(t, spec.MaxUnavailable)
}

func TestPodDisruptionBudgetSpecEditor_SetSelector(t *testing.T) {
	t.Parallel()
	spec := &policyv1.PodDisruptionBudgetSpec{}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "my-app"},
	}
	e.SetSelector(selector)

	require.NotNil(t, spec.Selector)
	assert.Equal(t, selector, spec.Selector)
}

func TestPodDisruptionBudgetSpecEditor_SetUnhealthyPodEvictionPolicy(t *testing.T) {
	t.Parallel()
	spec := &policyv1.PodDisruptionBudgetSpec{}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	e.SetUnhealthyPodEvictionPolicy(policyv1.AlwaysAllow)

	require.NotNil(t, spec.UnhealthyPodEvictionPolicy)
	assert.Equal(t, policyv1.AlwaysAllow, *spec.UnhealthyPodEvictionPolicy)
}

func TestPodDisruptionBudgetSpecEditor_ClearUnhealthyPodEvictionPolicy(t *testing.T) {
	t.Parallel()
	policy := policyv1.IfHealthyBudget
	spec := &policyv1.PodDisruptionBudgetSpec{UnhealthyPodEvictionPolicy: &policy}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	e.ClearUnhealthyPodEvictionPolicy()

	assert.Nil(t, spec.UnhealthyPodEvictionPolicy)
}

func TestPodDisruptionBudgetSpecEditor_Raw(t *testing.T) {
	t.Parallel()
	spec := &policyv1.PodDisruptionBudgetSpec{}
	e := NewPodDisruptionBudgetSpecEditor(spec)

	assert.Same(t, spec, e.Raw())
}
