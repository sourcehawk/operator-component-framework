package component

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadResources(t *testing.T) {
	var (
		scheme    = setupScheme()
		namespace = "test-namespace"
		owner     = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-owner",
				Namespace: namespace,
			},
		}
		fakeClient       = fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
		reconcileContext = setupReconcileContext(scheme, owner, fakeClient)
		ctx              = t.Context()
	)

	t.Run("should successfully read multiple resources", func(t *testing.T) {
		// Given
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: namespace,
			},
		}
		err := fakeClient.Create(ctx, cm)
		require.NoError(t, err)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: namespace,
			},
		}
		err = fakeClient.Create(ctx, secret)
		require.NoError(t, err)

		// Resource 1: not Alive
		resource1 := &MockResource{}
		resource1.On("Object").Return(cm, nil)

		// Resource 2: Alive
		resource2 := &MockAliveResource{}
		resource2.On("Object").Return(secret, nil)
		resource2.On("ConvergingStatus", ConvergingOperationNone).Return(ConvergingStatusWithReason{
			Status: ConvergingStatusReady,
			Reason: "Secret is ready",
		}, nil)

		// When
		results, err := readResources(ctx, reconcileContext, []Resource{resource1, resource2})

		// Then
		require.NoError(t, err)
		assert.Len(t, results, 1) // Only resource2 is Alive
		assert.Equal(t, resource2, results[0].Resource)
		assert.Equal(t, ConvergingStatusReady, results[0].Status.Status)

		resource1.AssertExpectations(t)
		resource2.AssertExpectations(t)
	})

	t.Run("should return error if resource.Object() fails", func(t *testing.T) {
		// Given
		resource := &MockResource{}
		resource.On("Object").Return(nil, errors.New("failed to get object"))
		resource.On("Identity").Return("v1/ConfigMap/failed-resource")

		// When
		results, err := readResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "failed to retrieve read-only object from resource v1/ConfigMap/failed-resource: failed to get object")

		resource.AssertExpectations(t)
	})

	t.Run("should return error and update status if client.Get() fails", func(t *testing.T) {
		// Given
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-cm",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(cm, nil)
		resource.On("Identity").Return("v1/ConfigMap/missing-cm")

		// When
		results, err := readResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.Error(t, err)
		assert.Nil(t, results)
		assert.True(t, apierrors.IsNotFound(errors.Unwrap(err)))

		resource.AssertExpectations(t)
	})

	t.Run("should return error and update status if alive.ConvergingStatus() fails", func(t *testing.T) {
		// Given
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-alive-fail",
				Namespace: namespace,
			},
		}
		err := fakeClient.Create(ctx, cm)
		require.NoError(t, err)

		resource := &MockAliveResource{}
		resource.On("Object").Return(cm, nil)
		resource.On("Identity").Return("v1/ConfigMap/test-cm-alive-fail")
		resource.On("ConvergingStatus", ConvergingOperationNone).Return(ConvergingStatusWithReason{}, errors.New("failed status"))

		// When
		results, err := readResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "failed status")

		resource.AssertExpectations(t)
	})
}
