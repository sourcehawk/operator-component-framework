package serviceaccount

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidSA() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "test-ns",
			Labels:    map[string]string{"app": "test"},
		},
	}
}

// --- Resource.Identity tests ---

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidSA()).Build()
	require.NoError(t, err)

	assert.Equal(t, "v1/ServiceAccount/test-ns/test-sa", res.Identity())
}

// --- Resource.Object tests ---

func TestResource_Object_ReturnsDeepCopy(t *testing.T) {
	sa := newValidSA()
	res, err := NewBuilder(sa).Build()
	require.NoError(t, err)

	got, err := res.Object()
	require.NoError(t, err)

	casted, ok := got.(*corev1.ServiceAccount)
	require.True(t, ok)
	assert.Equal(t, "test-sa", casted.Name)

	// Mutating the returned copy must not affect the original
	casted.Name = "changed"
	assert.Equal(t, "test-sa", sa.Name)

	// A second call should also be independent
	got2, err := res.Object()
	require.NoError(t, err)
	assert.Equal(t, "test-sa", got2.GetName())
}

// --- Resource.Mutate tests ---

func TestResource_Mutate_AppliesDesiredState(t *testing.T) {
	desired := newValidSA()
	desired.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "registry"}}

	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.ServiceAccount)
	assert.Equal(t, "test", got.Labels["app"])
	require.Len(t, got.ImagePullSecrets, 1)
	assert.Equal(t, "registry", got.ImagePullSecrets[0].Name)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidSA()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-pull-secret",
			Feature: feature.NewVersionGate("1.0.0", nil),
			Mutate: func(m *Mutator) error {
				m.EnsureImagePullSecret("my-registry")
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.ServiceAccount)
	require.Len(t, got.ImagePullSecrets, 1)
	assert.Equal(t, "my-registry", got.ImagePullSecrets[0].Name)
}

// --- Resource.ExtractData tests ---

func TestResource_ExtractData(t *testing.T) {
	sa := newValidSA()
	sa.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "reg"}}

	var extractedName string
	res, err := NewBuilder(sa).
		WithDataExtractor(func(s corev1.ServiceAccount) error {
			extractedName = s.ImagePullSecrets[0].Name
			return nil
		}).
		Build()
	require.NoError(t, err)

	require.NoError(t, res.ExtractData())
	assert.Equal(t, "reg", extractedName)
}

func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidSA()).
		WithDataExtractor(func(_ corev1.ServiceAccount) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}
