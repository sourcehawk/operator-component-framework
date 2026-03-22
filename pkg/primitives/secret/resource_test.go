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

	current := &corev1.Secret{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, []byte("value"), current.Data["key"])
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

	current := &corev1.Secret{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, []byte("value"), current.Data["key"])
	assert.Equal(t, []byte("yes"), current.Data["from-mutation"])
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	// A second mutation should observe changes made by the first.
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

	current := &corev1.Secret{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, []byte("b"), current.Data["order"])
}

func TestResource_Mutate_CustomFieldApplicator(t *testing.T) {
	desired := newValidSecret()

	applicatorCalled := false
	res, err := NewBuilder(desired).
		WithCustomFieldApplicator(func(current, d *corev1.Secret) error {
			applicatorCalled = true
			// Only copy "key", ignore everything else.
			if current.Data == nil {
				current.Data = make(map[string][]byte)
			}
			current.Data["key"] = d.Data["key"]
			return nil
		}).
		Build()
	require.NoError(t, err)

	current := &corev1.Secret{
		Data: map[string][]byte{"external": []byte("preserved")},
	}
	require.NoError(t, res.Mutate(current))

	assert.True(t, applicatorCalled)
	assert.Equal(t, []byte("value"), current.Data["key"])
	assert.Equal(t, []byte("preserved"), current.Data["external"])
}

func TestResource_Mutate_CustomFieldApplicator_Error(t *testing.T) {
	res, err := NewBuilder(newValidSecret()).
		WithCustomFieldApplicator(func(_, _ *corev1.Secret) error {
			return errors.New("applicator error")
		}).
		Build()
	require.NoError(t, err)

	err = res.Mutate(&corev1.Secret{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applicator error")
}

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	current := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			Namespace:       "default",
			ResourceVersion: "12345",
			UID:             "abc-def",
			Generation:      3,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Pod", Name: "other-owner", UID: "other-uid"},
			},
			Finalizers: []string{"finalizer.example.com"},
		},
	}
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Data: map[string][]byte{"key": []byte("value")},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec and labels are applied
	assert.Equal(t, []byte("value"), current.Data["key"])
	assert.Equal(t, "test", current.Labels["app"])

	// Server-managed fields are preserved
	assert.Equal(t, "12345", current.ResourceVersion)
	assert.Equal(t, "abc-def", string(current.UID))
	assert.Equal(t, int64(3), current.Generation)

	// Shared-controller fields are preserved
	assert.Len(t, current.OwnerReferences, 1)
	assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)
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
