package configmap

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newHashTestCM(data map[string]string, binaryData map[string][]byte) corev1.ConfigMap {
	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       data,
		BinaryData: binaryData,
	}
}

// --- Determinism ---

func TestDataHash_Deterministic(t *testing.T) {
	cm := newHashTestCM(map[string]string{"key": "value"}, nil)
	h1, err := DataHash(cm)
	require.NoError(t, err)
	h2, err := DataHash(cm)
	require.NoError(t, err)
	assert.Equal(t, h1, h2)
}

func TestDataHash_InsertionOrderIndependent(t *testing.T) {
	// Maps in Go have non-deterministic iteration order; JSON encoding sorts keys.
	cm1 := newHashTestCM(map[string]string{"a": "1", "b": "2"}, nil)
	cm2 := newHashTestCM(map[string]string{"b": "2", "a": "1"}, nil)

	h1, err := DataHash(cm1)
	require.NoError(t, err)
	h2, err := DataHash(cm2)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "hash must be order-independent")
}

// --- Sensitivity ---

func TestDataHash_ChangesOnDataChange(t *testing.T) {
	cm1 := newHashTestCM(map[string]string{"key": "old"}, nil)
	cm2 := newHashTestCM(map[string]string{"key": "new"}, nil)

	h1, err := DataHash(cm1)
	require.NoError(t, err)
	h2, err := DataHash(cm2)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestDataHash_ChangesOnBinaryDataChange(t *testing.T) {
	cm1 := newHashTestCM(nil, map[string][]byte{"cert.pem": []byte("old")})
	cm2 := newHashTestCM(nil, map[string][]byte{"cert.pem": []byte("new")})

	h1, err := DataHash(cm1)
	require.NoError(t, err)
	h2, err := DataHash(cm2)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestDataHash_ChangesOnKeyChange(t *testing.T) {
	cm1 := newHashTestCM(map[string]string{"key-a": "value"}, nil)
	cm2 := newHashTestCM(map[string]string{"key-b": "value"}, nil)

	h1, err := DataHash(cm1)
	require.NoError(t, err)
	h2, err := DataHash(cm2)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestDataHash_DataAndBinaryDataContributeSeparately(t *testing.T) {
	// Same content in .data vs .binaryData must produce different hashes.
	cm1 := newHashTestCM(map[string]string{"key": "value"}, nil)
	cm2 := newHashTestCM(nil, map[string][]byte{"key": []byte("value")})

	h1, err := DataHash(cm1)
	require.NoError(t, err)
	h2, err := DataHash(cm2)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestDataHash_NilAndEmptyMapHashIdentically(t *testing.T) {
	// A ConfigMap with nil .data/.binaryData must hash the same as one with
	// explicitly empty maps — both represent "no entries".
	cmNil := newHashTestCM(nil, nil)
	cmEmpty := newHashTestCM(map[string]string{}, map[string][]byte{})

	hNil, err := DataHash(cmNil)
	require.NoError(t, err)
	hEmpty, err := DataHash(cmEmpty)
	require.NoError(t, err)

	assert.Equal(t, hNil, hEmpty, "nil and empty maps must produce the same hash")
}

// --- Edge cases ---

func TestDataHash_EmptyConfigMap(t *testing.T) {
	cm := newHashTestCM(nil, nil)
	h, err := DataHash(cm)
	require.NoError(t, err)
	assert.NotEmpty(t, h)
}

func TestDataHash_MetadataIgnored(t *testing.T) {
	// Labels, annotations, and other metadata must not affect the hash.
	cm1 := newHashTestCM(map[string]string{"key": "value"}, nil)
	cm2 := newHashTestCM(map[string]string{"key": "value"}, nil)
	cm2.Labels = map[string]string{"extra": "label"}
	cm2.Annotations = map[string]string{"extra": "annotation"}

	h1, err := DataHash(cm1)
	require.NoError(t, err)
	h2, err := DataHash(cm2)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "metadata must not influence the hash")
}

func TestDataHash_ReturnsHexString(t *testing.T) {
	cm := newHashTestCM(map[string]string{"k": "v"}, nil)
	h, err := DataHash(cm)
	require.NoError(t, err)
	// SHA-256 in hex is always 64 characters.
	assert.Len(t, h, 64)
	for _, c := range h {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"expected lowercase hex character, got %c", c)
	}
}

// --- DesiredHash ---

func newHashTestResource(t *testing.T, data map[string]string, mutations ...Mutation) *Resource {
	t.Helper()
	base := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       data,
	}
	b := NewBuilder(base)
	for _, m := range mutations {
		b.WithMutation(m)
	}
	r, err := b.Build()
	require.NoError(t, err)
	return r
}

func TestDesiredHash_Deterministic(t *testing.T) {
	r := newHashTestResource(t, map[string]string{"key": "value"})
	h1, err := r.DesiredHash()
	require.NoError(t, err)
	h2, err := r.DesiredHash()
	require.NoError(t, err)
	assert.Equal(t, h1, h2)
}

func TestDesiredHash_NoSideEffects(t *testing.T) {
	// Calling DesiredHash must not modify the resource's internal state.
	// A subsequent reconciliation must produce the same result as the first.
	r := newHashTestResource(t, nil, Mutation{
		Name:    "set-key",
		Feature: feature.NewVersionGate("1.0.0", nil),
		Mutate: func(m *Mutator) error {
			m.SetEntry("key", "value")
			return nil
		},
	})

	h1, err := r.DesiredHash()
	require.NoError(t, err)
	h2, err := r.DesiredHash()
	require.NoError(t, err)
	assert.Equal(t, h1, h2, "repeated DesiredHash calls must return the same value")

	// Verify the resource still reconciles correctly after hash computation.
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
	require.NoError(t, r.Mutate(cm))
	require.NoError(t, r.Mutate(cm))
	assert.Equal(t, "value", cm.Data["key"], "mutations must not be applied more than once per reconcile")
}

func TestDesiredHash_ChangesWhenMutationChangesContent(t *testing.T) {
	r1 := newHashTestResource(t, nil, Mutation{
		Name:    "set-key",
		Feature: feature.NewVersionGate("1.0.0", nil),
		Mutate:  func(m *Mutator) error { m.SetEntry("key", "v1"); return nil },
	})
	r2 := newHashTestResource(t, nil, Mutation{
		Name:    "set-key",
		Feature: feature.NewVersionGate("1.0.0", nil),
		Mutate:  func(m *Mutator) error { m.SetEntry("key", "v2"); return nil },
	})

	h1, err := r1.DesiredHash()
	require.NoError(t, err)
	h2, err := r2.DesiredHash()
	require.NoError(t, err)
	assert.NotEqual(t, h1, h2)
}

func TestDesiredHash_DisabledMutationDoesNotAffectHash(t *testing.T) {
	base := newHashTestResource(t, nil, Mutation{
		Name:    "always",
		Feature: feature.NewVersionGate("1.0.0", nil),
		Mutate:  func(m *Mutator) error { m.SetEntry("key", "value"); return nil },
	})

	withDisabled := newHashTestResource(t, nil,
		Mutation{
			Name:    "always",
			Feature: feature.NewVersionGate("1.0.0", nil),
			Mutate:  func(m *Mutator) error { m.SetEntry("key", "value"); return nil },
		},
		Mutation{
			Name:    "disabled",
			Feature: feature.NewVersionGate("1.0.0", nil).When(false),
			Mutate:  func(m *Mutator) error { m.SetEntry("extra", "skipped"); return nil },
		},
	)

	h1, err := base.DesiredHash()
	require.NoError(t, err)
	h2, err := withDisabled.DesiredHash()
	require.NoError(t, err)
	assert.Equal(t, h1, h2, "disabled mutation must not influence the hash")
}

func TestDesiredHash_MetadataOnlyMutationDoesNotAffectHash(t *testing.T) {
	withoutLabel := newHashTestResource(t, map[string]string{"key": "value"})
	withLabel := newHashTestResource(t, map[string]string{"key": "value"}, Mutation{
		Name:    "label",
		Feature: feature.NewVersionGate("1.0.0", nil),
		Mutate: func(m *Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", "1.0.0")
				return nil
			})
			return nil
		},
	})

	h1, err := withoutLabel.DesiredHash()
	require.NoError(t, err)
	h2, err := withLabel.DesiredHash()
	require.NoError(t, err)
	assert.Equal(t, h1, h2, "metadata-only mutations must not influence the hash")
}
