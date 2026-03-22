package secret

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newHashTestSecret(data map[string][]byte) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       data,
	}
}

// --- Determinism ---

func TestDataHash_Deterministic(t *testing.T) {
	s := newHashTestSecret(map[string][]byte{"key": []byte("value")})
	h1, err := DataHash(s)
	require.NoError(t, err)
	h2, err := DataHash(s)
	require.NoError(t, err)
	assert.Equal(t, h1, h2)
}

func TestDataHash_InsertionOrderIndependent(t *testing.T) {
	// Maps in Go have non-deterministic iteration order; JSON encoding sorts keys.
	s1 := newHashTestSecret(map[string][]byte{"a": []byte("1"), "b": []byte("2")})
	s2 := newHashTestSecret(map[string][]byte{"b": []byte("2"), "a": []byte("1")})

	h1, err := DataHash(s1)
	require.NoError(t, err)
	h2, err := DataHash(s2)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "hash must be order-independent")
}

// --- Sensitivity ---

func TestDataHash_ChangesOnDataChange(t *testing.T) {
	s1 := newHashTestSecret(map[string][]byte{"key": []byte("old")})
	s2 := newHashTestSecret(map[string][]byte{"key": []byte("new")})

	h1, err := DataHash(s1)
	require.NoError(t, err)
	h2, err := DataHash(s2)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestDataHash_ChangesOnKeyChange(t *testing.T) {
	s1 := newHashTestSecret(map[string][]byte{"key-a": []byte("value")})
	s2 := newHashTestSecret(map[string][]byte{"key-b": []byte("value")})

	h1, err := DataHash(s1)
	require.NoError(t, err)
	h2, err := DataHash(s2)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

// --- Edge cases ---

func TestDataHash_NilAndEmptyMapHashIdentically(t *testing.T) {
	sNil := newHashTestSecret(nil)
	sEmpty := newHashTestSecret(map[string][]byte{})

	hNil, err := DataHash(sNil)
	require.NoError(t, err)
	hEmpty, err := DataHash(sEmpty)
	require.NoError(t, err)

	assert.Equal(t, hNil, hEmpty, "nil and empty data must produce the same hash")
}

func TestDataHash_EmptySecret(t *testing.T) {
	s := newHashTestSecret(nil)
	h, err := DataHash(s)
	require.NoError(t, err)
	assert.NotEmpty(t, h)
}

func TestDataHash_MetadataIgnored(t *testing.T) {
	s1 := newHashTestSecret(map[string][]byte{"key": []byte("value")})
	s2 := newHashTestSecret(map[string][]byte{"key": []byte("value")})
	s2.Labels = map[string]string{"extra": "label"}
	s2.Annotations = map[string]string{"extra": "annotation"}

	h1, err := DataHash(s1)
	require.NoError(t, err)
	h2, err := DataHash(s2)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "metadata must not influence the hash")
}

func TestDataHash_ReturnsHexString(t *testing.T) {
	s := newHashTestSecret(map[string][]byte{"k": []byte("v")})
	h, err := DataHash(s)
	require.NoError(t, err)
	// SHA-256 in hex is always 64 characters.
	assert.Len(t, h, 64)
	for _, c := range h {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"expected lowercase hex character, got %c", c)
	}
}

// --- StringData merging ---

func TestDataHash_StringDataIncluded(t *testing.T) {
	s := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		StringData: map[string]string{"key": "value"},
	}
	hSD, err := DataHash(s)
	require.NoError(t, err)

	sData := newHashTestSecret(map[string][]byte{"key": []byte("value")})
	hData, err := DataHash(sData)
	require.NoError(t, err)

	assert.Equal(t, hSD, hData, "stringData and equivalent data must produce the same hash")
}

func TestDataHash_StringDataOverridesData(t *testing.T) {
	s := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string][]byte{"key": []byte("from-data")},
		StringData: map[string]string{"key": "from-stringdata"},
	}
	h1, err := DataHash(s)
	require.NoError(t, err)

	sExpected := newHashTestSecret(map[string][]byte{"key": []byte("from-stringdata")})
	h2, err := DataHash(sExpected)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "stringData must override data for the same key")
}

// --- DesiredHash ---

func newHashTestResource(t *testing.T, data map[string][]byte, mutations ...Mutation) *Resource {
	t.Helper()
	base := &corev1.Secret{
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
	r := newHashTestResource(t, map[string][]byte{"key": []byte("value")})
	h1, err := r.DesiredHash()
	require.NoError(t, err)
	h2, err := r.DesiredHash()
	require.NoError(t, err)
	assert.Equal(t, h1, h2)
}

func TestDesiredHash_NoSideEffects(t *testing.T) {
	// Calling DesiredHash must not modify the resource's internal state.
	r := newHashTestResource(t, nil, Mutation{
		Name:    "set-key",
		Feature: feature.NewResourceFeature("1.0.0", nil),
		Mutate: func(m *Mutator) error {
			m.SetData("key", []byte("value"))
			return nil
		},
	})

	h1, err := r.DesiredHash()
	require.NoError(t, err)
	h2, err := r.DesiredHash()
	require.NoError(t, err)
	assert.Equal(t, h1, h2, "repeated DesiredHash calls must return the same value")

	// Verify the resource still reconciles correctly after hash computation.
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
	require.NoError(t, r.Mutate(s))
	require.NoError(t, r.Mutate(s))
	assert.Equal(t, []byte("value"), s.Data["key"], "mutations must not be applied more than once per reconcile")
}

func TestDesiredHash_ChangesWhenMutationChangesContent(t *testing.T) {
	r1 := newHashTestResource(t, nil, Mutation{
		Name:    "set-key",
		Feature: feature.NewResourceFeature("1.0.0", nil),
		Mutate:  func(m *Mutator) error { m.SetData("key", []byte("v1")); return nil },
	})
	r2 := newHashTestResource(t, nil, Mutation{
		Name:    "set-key",
		Feature: feature.NewResourceFeature("1.0.0", nil),
		Mutate:  func(m *Mutator) error { m.SetData("key", []byte("v2")); return nil },
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
		Feature: feature.NewResourceFeature("1.0.0", nil),
		Mutate:  func(m *Mutator) error { m.SetData("key", []byte("value")); return nil },
	})

	withDisabled := newHashTestResource(t, nil,
		Mutation{
			Name:    "always",
			Feature: feature.NewResourceFeature("1.0.0", nil),
			Mutate:  func(m *Mutator) error { m.SetData("key", []byte("value")); return nil },
		},
		Mutation{
			Name:    "disabled",
			Feature: feature.NewResourceFeature("1.0.0", nil).When(false),
			Mutate:  func(m *Mutator) error { m.SetData("extra", []byte("skipped")); return nil },
		},
	)

	h1, err := base.DesiredHash()
	require.NoError(t, err)
	h2, err := withDisabled.DesiredHash()
	require.NoError(t, err)
	assert.Equal(t, h1, h2, "disabled mutation must not influence the hash")
}
