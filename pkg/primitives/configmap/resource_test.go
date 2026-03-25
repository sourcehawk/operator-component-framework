package configmap

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "test-ns",
		},
		Data: map[string]string{"key": "value"},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidCM()).Build()
	require.NoError(t, err)
	assert.Equal(t, "v1/ConfigMap/test-ns/test-cm", res.Identity())
}

func TestResource_Object(t *testing.T) {
	cm := newValidCM()
	res, err := NewBuilder(cm).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*corev1.ConfigMap)
	require.True(t, ok)
	assert.Equal(t, cm.Name, got.Name)
	assert.Equal(t, cm.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-cm", cm.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidCM()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	cm := obj.(*corev1.ConfigMap)
	assert.Equal(t, "value", cm.Data["key"])
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidCM()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-entry",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetEntry("from-mutation", "yes")
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	cm := obj.(*corev1.ConfigMap)
	assert.Equal(t, "value", cm.Data["key"])
	assert.Equal(t, "yes", cm.Data["from-mutation"])
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	// A second mutation should observe changes made by the first.
	desired := newValidCM()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetEntry("order", "a")
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.SetEntry("order", "b")
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	current := &corev1.ConfigMap{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, "b", current.Data["order"])
}

func TestResource_ExtractData(t *testing.T) {
	cm := newValidCM()

	var extracted string
	res, err := NewBuilder(cm).
		WithDataExtractor(func(c corev1.ConfigMap) error {
			extracted = c.Data["key"]
			return nil
		}).
		Build()
	require.NoError(t, err)

	require.NoError(t, res.ExtractData())
	assert.Equal(t, "value", extracted)
}

func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidCM()).
		WithDataExtractor(func(_ corev1.ConfigMap) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}
