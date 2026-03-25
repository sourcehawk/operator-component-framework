package secret

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{"key": []byte("value")},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidSecret()).Build()
	require.NoError(t, err)
	assert.Equal(t, "v1/Secret/test-ns/test-secret", res.Identity())
}

func TestResource_Object(t *testing.T) {
	s := newValidSecret()
	res, err := NewBuilder(s).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*corev1.Secret)
	require.True(t, ok)
	assert.Equal(t, s.Name, got.Name)
	assert.Equal(t, s.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-secret", s.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidSecret()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.Secret)
	assert.Equal(t, []byte("value"), got.Data["key"])
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidSecret()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-entry",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetData("from-mutation", []byte("yes"))
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.Secret)
	assert.Equal(t, []byte("value"), got.Data["key"])
	assert.Equal(t, []byte("yes"), got.Data["from-mutation"])
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	// When two features write the same key, the last feature wins (deterministic ordering).
	desired := newValidSecret()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetData("order", []byte("a"))
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetData("order", []byte("b"))
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.Secret)
	assert.Equal(t, []byte("b"), got.Data["order"])
}

func TestResource_ExtractData(t *testing.T) {
	s := newValidSecret()

	var extracted []byte
	res, err := NewBuilder(s).
		WithDataExtractor(func(c corev1.Secret) error {
			extracted = c.Data["key"]
			return nil
		}).
		Build()
	require.NoError(t, err)

	require.NoError(t, res.ExtractData())
	assert.Equal(t, []byte("value"), extracted)
}

func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidSecret()).
		WithDataExtractor(func(_ corev1.Secret) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}
