package serviceaccount

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

// --- DefaultFieldApplicator tests ---

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	current := &corev1.ServiceAccount{
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
	desired := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired labels are applied
	assert.Equal(t, "test", current.Labels["app"])

	// Server-managed fields are preserved
	assert.Equal(t, "12345", current.ResourceVersion)
	assert.Equal(t, types.UID("abc-def"), current.UID)
	assert.Equal(t, int64(3), current.Generation)

	// Shared-controller fields are preserved
	assert.Len(t, current.OwnerReferences, 1)
	assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)
}

func TestDefaultFieldApplicator_PreservesSecrets(t *testing.T) {
	current := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Secrets: []corev1.ObjectReference{
			{Name: "token-abc", Namespace: "default"},
			{Name: "token-xyz", Namespace: "default"},
		},
	}
	desired := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Secrets from the live object are preserved
	require.Len(t, current.Secrets, 2)
	assert.Equal(t, "token-abc", current.Secrets[0].Name)
	assert.Equal(t, "token-xyz", current.Secrets[1].Name)
}

func TestDefaultFieldApplicator_DesiredSecretsAreDiscarded(t *testing.T) {
	current := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Secrets: []corev1.ObjectReference{
			{Name: "live-token", Namespace: "default"},
		},
	}
	desired := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Secrets: []corev1.ObjectReference{
			{Name: "desired-token", Namespace: "default"},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// The live secrets are preserved; desired secrets are discarded
	require.Len(t, current.Secrets, 1)
	assert.Equal(t, "live-token", current.Secrets[0].Name)
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

	current := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sa",
			Namespace:       "test-ns",
			ResourceVersion: "999",
		},
	}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, "test", current.Labels["app"])
	require.Len(t, current.ImagePullSecrets, 1)
	assert.Equal(t, "registry", current.ImagePullSecrets[0].Name)
	// Server-managed field preserved
	assert.Equal(t, "999", current.ResourceVersion)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidSA()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-pull-secret",
			Feature: feature.NewResourceFeature("1.0.0", nil),
			Mutate: func(m *Mutator) error {
				m.EnsureImagePullSecret("my-registry")
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	current := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "test-ns",
		},
	}
	require.NoError(t, res.Mutate(current))

	require.Len(t, current.ImagePullSecrets, 1)
	assert.Equal(t, "my-registry", current.ImagePullSecrets[0].Name)
}

func TestResource_Mutate_WithFlavor(t *testing.T) {
	desired := newValidSA()
	desired.Labels = map[string]string{"app": "test"}

	res, err := NewBuilder(desired).
		WithFieldApplicationFlavor(PreserveCurrentLabels).
		Build()
	require.NoError(t, err)

	current := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "test-ns",
			Labels:    map[string]string{"external": "preserved", "app": "old"},
		},
	}
	require.NoError(t, res.Mutate(current))

	// Desired label wins on overlap
	assert.Equal(t, "test", current.Labels["app"])
	// External label is preserved
	assert.Equal(t, "preserved", current.Labels["external"])
}

func TestResource_Mutate_CustomFieldApplicator(t *testing.T) {
	desired := newValidSA()
	desired.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "desired-secret"}}

	applicatorCalled := false
	res, err := NewBuilder(desired).
		WithCustomFieldApplicator(func(current, d *corev1.ServiceAccount) error {
			applicatorCalled = true
			current.ImagePullSecrets = d.ImagePullSecrets
			return nil
		}).
		Build()
	require.NoError(t, err)

	current := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "test-ns",
			Labels:    map[string]string{"external": "should-stay"},
		},
	}
	require.NoError(t, res.Mutate(current))

	assert.True(t, applicatorCalled)
	require.Len(t, current.ImagePullSecrets, 1)
	assert.Equal(t, "desired-secret", current.ImagePullSecrets[0].Name)
	// Custom applicator didn't touch labels, so they stay
	assert.Equal(t, "should-stay", current.Labels["external"])
}

func TestResource_Mutate_CustomFieldApplicator_Error(t *testing.T) {
	res, err := NewBuilder(newValidSA()).
		WithCustomFieldApplicator(func(_, _ *corev1.ServiceAccount) error {
			return errors.New("applicator error")
		}).
		Build()
	require.NoError(t, err)

	err = res.Mutate(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sa", Namespace: "test-ns"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applicator error")
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
