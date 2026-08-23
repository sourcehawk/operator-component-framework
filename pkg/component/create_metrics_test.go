package component

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// identifiedResource is an operationRecordingResource that carries a metrics
// identifier, exercising the concepts.MetricsIdentifiable path.
type identifiedResource struct {
	operationRecordingResource
	identifier string
}

func (r *identifiedResource) MetricsIdentifier() string { return r.identifier }

// failingPatchClient fails every Patch and leaves every other operation intact,
// so an apply reaches the patch before it fails.
type failingPatchClient struct {
	client.Client
}

func (c failingPatchClient) Patch(
	context.Context, client.Object, client.Patch, ...client.PatchOption,
) error {
	return errors.New("patch rejected")
}

// objectlessResource fails to produce an object at all, which happens before
// the framework can determine a kind to label metrics with.
type objectlessResource struct {
	operationRecordingResource
}

func (r *objectlessResource) Object() (client.Object, error) {
	return nil, errors.New("cannot build object")
}

func TestApplyResourceMetrics(t *testing.T) {
	const namespace = "test-namespace"

	newEnv := func(t *testing.T) ReconcileContext {
		t.Helper()
		scheme := setupScheme()
		owner := &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: namespace, UID: "owner-uid"},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).WithObjects(owner).WithStatusSubresource(owner).Build()
		return setupReconcileContext(scheme, owner, fakeClient)
	}

	buildConfigMap := func() *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "rebuilt-cm", Namespace: namespace},
			Data:       map[string]string{"foo": "bar"},
		}
	}

	apply := func(t *testing.T, rec ReconcileContext, res Resource) error {
		t.Helper()
		_, err := applyResources(
			t.Context(), rec, []reconcileEntry{{Resource: res}}, "test-component", createTestRESTMapper(),
		)
		return err
	}

	t.Run("records one apply per pass, defaulting the identifier to the lowercased kind", func(t *testing.T) {
		rec := newEnv(t)
		res := &operationRecordingResource{build: buildConfigMap}

		require.NoError(t, apply(t, rec, res))
		require.NoError(t, apply(t, rec, res))

		applies := rec.Metrics.(*spyMetrics).recordedApplies()
		require.Len(t, applies, 2)
		assert.Equal(t, concepts.ConvergingOperationCreated, applies[0].operation)
		assert.Equal(t, concepts.ConvergingOperationNone, applies[1].operation)
		assert.Equal(t, ResourceMetricLabels{
			OwnerKind:  "MockOperatorCRD",
			Component:  "test-component",
			Identifier: "configmap",
			Kind:       "ConfigMap",
		}, applies[0].labels)
		assert.Empty(t, rec.Metrics.(*spyMetrics).recordedErrors())
	})

	t.Run("uses the resource's identifier when it declares one", func(t *testing.T) {
		rec := newEnv(t)
		res := &identifiedResource{
			operationRecordingResource: operationRecordingResource{build: buildConfigMap},
			identifier:                 "tls",
		}

		require.NoError(t, apply(t, rec, res))

		applies := rec.Metrics.(*spyMetrics).recordedApplies()
		require.Len(t, applies, 1)
		assert.Equal(t, "tls", applies[0].labels.Identifier)
		assert.Equal(t, "ConfigMap", applies[0].labels.Kind)
	})

	t.Run("falls back to the kind when the resource declares an empty identifier", func(t *testing.T) {
		rec := newEnv(t)
		res := &identifiedResource{
			operationRecordingResource: operationRecordingResource{build: buildConfigMap},
			identifier:                 "",
		}

		require.NoError(t, apply(t, rec, res))

		applies := rec.Metrics.(*spyMetrics).recordedApplies()
		require.Len(t, applies, 1)
		assert.Equal(t, "configmap", applies[0].labels.Identifier)
	})

	t.Run("falls back to the kind when the resource declares a whitespace-only identifier", func(t *testing.T) {
		// The builders reject a blank identifier, but a hand-written
		// concepts.MetricsIdentifiable can return one, and " " is not a label
		// value anyone meant to key a series by.
		rec := newEnv(t)
		res := &identifiedResource{
			operationRecordingResource: operationRecordingResource{build: buildConfigMap},
			identifier:                 "  \t ",
		}

		require.NoError(t, apply(t, rec, res))

		applies := rec.Metrics.(*spyMetrics).recordedApplies()
		require.Len(t, applies, 1)
		assert.Equal(t, "configmap", applies[0].labels.Identifier)
	})

	t.Run("records an error and no apply when the patch fails", func(t *testing.T) {
		rec := newEnv(t)
		rec.Client = failingPatchClient{Client: rec.Client}
		res := &operationRecordingResource{build: buildConfigMap}

		require.Error(t, apply(t, rec, res))

		spy := rec.Metrics.(*spyMetrics)
		assert.Empty(t, spy.recordedApplies())
		assert.Equal(t, []ResourceMetricLabels{{
			OwnerKind:  "MockOperatorCRD",
			Component:  "test-component",
			Identifier: "configmap",
			Kind:       "ConfigMap",
		}}, spy.recordedErrors())
	})

	t.Run("records nothing when the object cannot be built, since no kind is known", func(t *testing.T) {
		rec := newEnv(t)
		res := &objectlessResource{}

		require.Error(t, apply(t, rec, res))

		spy := rec.Metrics.(*spyMetrics)
		assert.Empty(t, spy.recordedApplies())
		assert.Empty(t, spy.recordedErrors())
	})

	t.Run("emits nothing when no recorder is configured", func(t *testing.T) {
		rec := newEnv(t)
		rec.Metrics = nil
		res := &operationRecordingResource{build: buildConfigMap}

		assert.NotPanics(t, func() { require.NoError(t, apply(t, rec, res)) })
	})

	t.Run("emits nothing for a read-only resource", func(t *testing.T) {
		rec := newEnv(t)
		require.NoError(t, rec.Client.Create(t.Context(), buildConfigMap()))
		res := &observableConfigMapResource{name: "rebuilt-cm", namespace: namespace}

		_, err := reconcileResources(
			t.Context(), rec,
			[]reconcileEntry{{Resource: res, Options: resourceOptions{ReadOnly: true}}},
			"test-component", createTestRESTMapper(),
		)
		require.NoError(t, err)

		spy := rec.Metrics.(*spyMetrics)
		assert.Empty(t, spy.recordedApplies())
		assert.Empty(t, spy.recordedErrors())
	})
}

// observableConfigMapResource is a read-only resource: the framework fetches it
// and never applies it, so it must never produce apply metrics.
type observableConfigMapResource struct {
	name      string
	namespace string
}

func (r *observableConfigMapResource) Identity() string {
	return "v1/ConfigMap/" + r.namespace + "/" + r.name
}

func (r *observableConfigMapResource) Object() (client.Object, error) {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: r.name, Namespace: r.namespace},
	}, nil
}

func (r *observableConfigMapResource) Mutate(client.Object) error { return nil }
