package component

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDeleteResources(t *testing.T) {
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

	t.Run("should successfully delete a resource", func(t *testing.T) {
		// Given
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: namespace,
			},
		}
		// Create the resource first so it can be deleted
		err := fakeClient.Create(ctx, resourceObject)
		require.NoError(t, err)

		resource := &MockResource{}
		resource.On("Object").Return(resourceObject, nil)
		resource.On("Identity").Return("v1/ConfigMap/test-cm")

		// When
		err = deleteResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.NoError(t, err)

		// Check if the resource is gone
		deletedConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-cm", Namespace: namespace}, deletedConfigMap)
		assert.True(t, apierrors.IsNotFound(err))

		resource.AssertExpectations(t)
	})

	t.Run("should ignore NotFound errors", func(t *testing.T) {
		// Given
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "non-existent",
				Namespace: namespace,
			},
		}
		// Resource is NOT created in fakeClient

		resource := &MockResource{}
		resource.On("Object").Return(resourceObject, nil)

		// When
		err := deleteResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.NoError(t, err)
		resource.AssertExpectations(t)
	})

	t.Run("should collect errors from Resource.Object() and continue", func(t *testing.T) {
		// Given
		resource1 := &MockResource{}
		resource1.On("Object").Return(nil, errors.New("failed to get object"))
		resource1.On("Identity").Return("v1/ConfigMap/failed-resource")

		resourceObject2 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-2",
				Namespace: namespace,
			},
		}
		err := fakeClient.Create(ctx, resourceObject2)
		require.NoError(t, err)

		resource2 := &MockResource{}
		resource2.On("Object").Return(resourceObject2, nil)
		resource2.On("Identity").Return("v1/ConfigMap/test-cm-2")

		// When
		err = deleteResources(ctx, reconcileContext, []Resource{resource1, resource2})

		// Then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get resource v1/ConfigMap/failed-resource's underlying object on deletion")

		// Check if resource2 was still deleted
		deletedConfigMap := &corev1.ConfigMap{}
		err2 := fakeClient.Get(ctx, client.ObjectKey{Name: "test-cm-2", Namespace: namespace}, deletedConfigMap)
		assert.True(t, apierrors.IsNotFound(err2))

		resource1.AssertExpectations(t)
		resource2.AssertExpectations(t)
	})

	t.Run("should collect deletion errors from client and continue", func(t *testing.T) {
		// Given
		resourceObject1 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-1",
				Namespace: namespace,
			},
		}

		// Let's use a mock client for this specific test case.
		mockClient := &MockClient{}
		recCtx := reconcileContext
		recCtx.Client = mockClient

		mockClient.On("Delete", ctx, resourceObject1, mock.Anything).Return(errors.New("forbidden"))

		resource1 := &MockResource{}
		resource1.On("Object").Return(resourceObject1, nil)
		resource1.On("Identity").Return("v1/ConfigMap/test-cm-1")

		resourceObject2 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-2",
				Namespace: namespace,
			},
		}
		mockClient.On("Delete", ctx, resourceObject2, mock.Anything).Return(nil)

		resource2 := &MockResource{}
		resource2.On("Object").Return(resourceObject2, nil)
		resource2.On("Identity").Return("v1/ConfigMap/test-cm-2")

		// When
		err := deleteResources(ctx, recCtx, []Resource{resource1, resource2})

		// Then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete resource v1/ConfigMap/test-cm-1: forbidden")

		mockClient.AssertExpectations(t)
		resource1.AssertExpectations(t)
		resource2.AssertExpectations(t)
	})
}
