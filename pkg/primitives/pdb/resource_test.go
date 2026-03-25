package pdb

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func newValidPDB() *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pdb",
			Namespace: "test-ns",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: ptr.To(intstr.FromInt32(2)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
		},
	}
}
func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidPDB()).Build()
	require.NoError(t, err)
	assert.Equal(t, "policy/v1/PodDisruptionBudget/test-ns/test-pdb", res.Identity())
}
func TestResource_Object(t *testing.T) {
	p := newValidPDB()
	res, err := NewBuilder(p).Build()
	require.NoError(t, err)
	obj, err := res.Object()
	require.NoError(t, err)
	got, ok := obj.(*policyv1.PodDisruptionBudget)
	require.True(t, ok)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, p.Namespace, got.Namespace)
	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-pdb", p.Name)
}
func TestResource_Mutate(t *testing.T) {
	desired := newValidPDB()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)
	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))
	got := obj.(*policyv1.PodDisruptionBudget)
	assert.Equal(t, intstr.FromInt32(2), *got.Spec.MinAvailable)
}
func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidPDB()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "set-max-unavailable",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				return m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
					e.SetMaxUnavailable(intstr.FromInt32(1))
					return nil
				})
			},
		}).
		Build()
	require.NoError(t, err)
	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))
	got := obj.(*policyv1.PodDisruptionBudget)
	require.NotNil(t, got.Spec.MaxUnavailable)
	assert.Equal(t, intstr.FromInt32(1), *got.Spec.MaxUnavailable)
}
func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidPDB()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				return m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
					e.EnsureLabel("order", "a")
					return nil
				})
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				return m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
					e.EnsureLabel("order", "b")
					return nil
				})
			},
		}).
		Build()
	require.NoError(t, err)
	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))
	got := obj.(*policyv1.PodDisruptionBudget)
	assert.Equal(t, "b", got.Labels["order"])
}
func TestResource_ExtractData(t *testing.T) {
	p := newValidPDB()
	var extracted int32
	res, err := NewBuilder(p).
		WithDataExtractor(func(pdb policyv1.PodDisruptionBudget) error {
			extracted = pdb.Spec.MinAvailable.IntVal
			return nil
		}).
		Build()
	require.NoError(t, err)
	require.NoError(t, res.ExtractData())
	assert.Equal(t, int32(2), extracted)
}
func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidPDB()).
		WithDataExtractor(func(_ policyv1.PodDisruptionBudget) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)
	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}
