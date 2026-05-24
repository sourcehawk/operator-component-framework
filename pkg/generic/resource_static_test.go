package generic

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStaticResource(t *testing.T) {
	const testVal = "bar"
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{"foo": testVal},
	}
	identityFunc := func(cm *corev1.ConfigMap) string { return cm.Name }
	newMutator := func(_ *corev1.ConfigMap) *mockMutator { return &mockMutator{} }

	res := &StaticResource[*corev1.ConfigMap, *mockMutator]{
		BaseResource: BaseResource[*corev1.ConfigMap, *mockMutator]{
			DesiredObject: obj,
			IdentityFunc:  identityFunc,
			NewMutator:    newMutator,
		},
	}

	t.Run("Identity", func(t *testing.T) {
		assert.Equal(t, "test-cm", res.Identity())
	})

	t.Run("Object", func(t *testing.T) {
		got, err := res.Object()
		require.NoError(t, err)
		assert.Equal(t, "test-cm", got.GetName())
		assert.NotSame(t, res.DesiredObject, got, "Object() should return a deep copy, but got same pointer")
	})

	t.Run("Mutate", func(t *testing.T) {
		got, err := res.Object()
		require.NoError(t, err)
		require.NoError(t, res.Mutate(got))
		cm := got.(*corev1.ConfigMap)
		assert.Equal(t, testVal, cm.Data["foo"])
	})

	t.Run("Mutate applies registered mutations", func(t *testing.T) {
		applied := false
		res.Mutations = nil
		res.Mutations = append(res.Mutations, mockMutation(func(_ *mockMutator) error {
			applied = true
			return nil
		}))

		got, err := res.Object()
		require.NoError(t, err)
		require.NoError(t, res.Mutate(got))
		assert.True(t, applied, "mutation was not applied")

		res.Mutations = nil
	})

	t.Run("ExtractData", func(t *testing.T) {
		extracted := false
		res.DataExtractors = []func(*corev1.ConfigMap) error{
			func(cm *corev1.ConfigMap) error {
				extracted = true
				assert.Equal(t, testVal, cm.Data["foo"])
				return nil
			},
		}
		err := res.ExtractData()
		require.NoError(t, err)
		assert.True(t, extracted, "extractor was not called")
	})

	t.Run("ExtractData error", func(t *testing.T) {
		res.DataExtractors = []func(*corev1.ConfigMap) error{
			func(_ *corev1.ConfigMap) error {
				return errors.New("extract error")
			},
		}
		err := res.ExtractData()
		assert.EqualError(t, err, "extract error")
	})

	t.Run("RecordObservation makes the observed object visible to ExtractData", func(t *testing.T) {
		base := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "base-cm",
				Namespace: "default",
			},
		}
		readOnly := &StaticResource[*corev1.ConfigMap, *mockMutator]{
			BaseResource: BaseResource[*corev1.ConfigMap, *mockMutator]{
				DesiredObject: base,
				IdentityFunc:  identityFunc,
				NewMutator:    newMutator,
			},
		}

		observed := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "base-cm",
				Namespace: "default",
			},
			Data: map[string]string{"foo": "from-cluster"},
		}
		require.NoError(t, readOnly.RecordObservation(observed))

		var seen string
		readOnly.DataExtractors = []func(*corev1.ConfigMap) error{
			func(cm *corev1.ConfigMap) error {
				seen = cm.Data["foo"]
				return nil
			},
		}
		require.NoError(t, readOnly.ExtractData())
		assert.Equal(t, "from-cluster", seen,
			"extractor must see the observed cluster object, not the empty desired base")
	})

	t.Run("RecordObservation rejects an object of the wrong type", func(t *testing.T) {
		readOnly := &StaticResource[*corev1.ConfigMap, *mockMutator]{
			BaseResource: BaseResource[*corev1.ConfigMap, *mockMutator]{
				DesiredObject: &corev1.ConfigMap{},
				IdentityFunc:  identityFunc,
				NewMutator:    newMutator,
			},
		}

		err := readOnly.RecordObservation(&corev1.Secret{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ConfigMap")
	})

	t.Run("GuardStatus returns unblocked when no handler is set", func(t *testing.T) {
		res.GuardHandler = nil
		result, err := res.GuardStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.GuardStatusUnblocked, result.Status)
	})

	t.Run("GuardStatus delegates to handler", func(t *testing.T) {
		res.GuardHandler = func(cm *corev1.ConfigMap) (concepts.GuardStatusWithReason, error) {
			if cm.Data["foo"] == "" {
				return concepts.GuardStatusWithReason{
					Status: concepts.GuardStatusBlocked,
					Reason: "foo is empty",
				}, nil
			}
			return concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusUnblocked,
			}, nil
		}

		result, err := res.GuardStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.GuardStatusUnblocked, result.Status)
	})

	t.Run("GuardStatus returns blocked from handler", func(t *testing.T) {
		res.GuardHandler = func(_ *corev1.ConfigMap) (concepts.GuardStatusWithReason, error) {
			return concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusBlocked,
				Reason: "waiting for dependency",
			}, nil
		}

		result, err := res.GuardStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.GuardStatusBlocked, result.Status)
		assert.Equal(t, "waiting for dependency", result.Reason)
	})

	t.Run("GuardStatus propagates handler errors", func(t *testing.T) {
		res.GuardHandler = func(_ *corev1.ConfigMap) (concepts.GuardStatusWithReason, error) {
			return concepts.GuardStatusWithReason{}, errors.New("guard error")
		}

		_, err := res.GuardStatus()
		assert.EqualError(t, err, "guard error")
	})
}
