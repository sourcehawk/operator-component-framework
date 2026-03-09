package component

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateOrUpdateResources(t *testing.T) {
	var (
		scheme          = setupScheme()
		namespace       = "test-namespace"
		ownerGeneration = int64(1)
		owner           = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-owner",
				Namespace:  namespace,
				Generation: ownerGeneration,
			},
		}
		fakeClient       = fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).WithStatusSubresource(owner).Build()
		reconcileContext = setupReconcileContext(scheme, owner, fakeClient)
		ctx              = t.Context()
	)

	t.Run("should successfully create a resource", func(t *testing.T) {
		// Given
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(resourceObject, nil)
		resource.On("Mutate").Return(nil)

		// When
		results, err := createOrUpdateResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.NoError(t, err)
		assert.Empty(t, results)

		createdConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-cm", Namespace: namespace}, createdConfigMap)
		require.NoError(t, err)
		assert.Len(t, createdConfigMap.OwnerReferences, 1)
		assert.Equal(t, owner.Name, createdConfigMap.OwnerReferences[0].Name)
		resource.AssertExpectations(t)
	})

	t.Run("should successfully create multiple resources", func(t *testing.T) {
		// Given
		resourceObject1 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-1",
				Namespace: namespace,
			},
		}
		resource1 := &MockResource{}
		resource1.On("Object").Return(resourceObject1, nil)
		resource1.On("Mutate").Return(nil)

		resourceObject2 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-2",
				Namespace: namespace,
			},
		}
		resource2 := &MockResource{}
		resource2.On("Object").Return(resourceObject2, nil)
		resource2.On("Mutate").Return(nil)

		// When
		results, err := createOrUpdateResources(ctx, reconcileContext, []Resource{resource1, resource2})

		// Then
		require.NoError(t, err)
		assert.Empty(t, results)

		for _, name := range []string{"test-cm-1", "test-cm-2"} {
			createdConfigMap := &corev1.ConfigMap{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, createdConfigMap)
			require.NoError(t, err)
			assert.Len(t, createdConfigMap.OwnerReferences, 1)
			assert.Equal(t, owner.Name, createdConfigMap.OwnerReferences[0].Name)
		}
		resource1.AssertExpectations(t)
		resource2.AssertExpectations(t)
	})

	t.Run("should successfully handle mixed regular and Alive resources", func(t *testing.T) {
		// Given
		regularResourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-regular",
				Namespace: namespace,
			},
		}
		regularResource := &MockResource{}
		regularResource.On("Object").Return(regularResourceObject, nil)
		regularResource.On("Mutate").Return(nil)

		aliveResourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-alive-mixed",
				Namespace: namespace,
			},
		}
		aliveResource := &MockAliveResource{}
		aliveResource.On("Object").Return(aliveResourceObject, nil)
		aliveResource.On("Mutate").Return(nil)
		aliveResource.On("ConvergingStatus", mock.Anything).Return(ConvergingStatusWithReason{
			Status: ConvergingStatusReady,
			Reason: "Ready",
		}, nil)

		// When
		results, err := createOrUpdateResources(ctx, reconcileContext, []Resource{regularResource, aliveResource})

		// Then
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, aliveResource, results[0].Resource)
		assert.Equal(t, ConvergingStatusReady, results[0].Status.Status)

		regularResource.AssertExpectations(t)
		aliveResource.AssertExpectations(t)
	})

	t.Run("should stop processing and return error on the first failure", func(t *testing.T) {
		// Given
		resourceObject1 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-success",
				Namespace: namespace,
			},
		}
		resource1 := &MockResource{}
		resource1.On("Object").Return(resourceObject1, nil)
		resource1.On("Mutate").Return(nil)

		resource2 := &MockResource{}
		resource2.On("Identity").Return("v1/ConfigMap/failed-resource")
		resource2.On("Object").Return(nil, fmt.Errorf("resource 2 error"))

		resource3 := &MockResource{} // Should not be processed

		// When
		results, err := createOrUpdateResources(ctx, reconcileContext, []Resource{resource1, resource2, resource3})

		// Then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource 2 error")
		assert.Nil(t, results)

		// Verify first resource was created
		createdConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-cm-success", Namespace: namespace}, createdConfigMap)
		require.NoError(t, err)

		resource1.AssertExpectations(t)
		resource2.AssertExpectations(t)
		// Explicitly verify subsequent resources are not touched
		resource3.AssertNotCalled(t, "Object")
	})

	t.Run("should successfully update a resource", func(t *testing.T) {
		// Given
		configMapName := "test-cm-update"
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: namespace,
			},
			Data: map[string]string{"foo": "bar"},
		}
		require.NoError(t, fakeClient.Create(ctx, configMap))

		updatedResourceObject := configMap.DeepCopy()
		updatedResourceObject.Data["foo"] = "baz"

		resource := &MockResource{}
		resource.On("Object").Return(configMap, nil)
		resource.On("Mutate").Run(func(_ mock.Arguments) {
			configMap.Data["foo"] = "baz"
		}).Return(nil)

		// When
		results, err := createOrUpdateResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.NoError(t, err)
		assert.Empty(t, results)

		updatedConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, updatedConfigMap)
		require.NoError(t, err)
		assert.Equal(t, "baz", updatedConfigMap.Data["foo"])
		resource.AssertExpectations(t)
	})

	t.Run("should handle Alive resources", func(t *testing.T) {
		// Given
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-alive",
				Namespace: namespace,
			},
		}
		resource := &MockAliveResource{}
		resource.On("Object").Return(resourceObject, nil)
		resource.On("Mutate").Return(nil)
		resource.On("ConvergingStatus", mock.Anything).Return(ConvergingStatusWithReason{
			Status: ConvergingStatusReady,
			Reason: "Ready",
		}, nil)

		// When
		results, err := createOrUpdateResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, resource, results[0].Resource)
		assert.Equal(t, ConvergingStatusReady, results[0].Status.Status)
		resource.AssertExpectations(t)
	})

	t.Run("should return error if resource.Object() fails", func(t *testing.T) {
		// Given
		resource := &MockResource{}
		resource.On("Identity").Return("v1/ConfigMap/failed-resource")
		resource.On("Object").Return(nil, fmt.Errorf("object error"))

		// When
		_, err := createOrUpdateResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get retrieve object")
		resource.AssertExpectations(t)
	})

	t.Run("should fail on mutation error", func(t *testing.T) {
		// Given
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "error-cm",
			},
		}
		resource := &MockResource{}
		resource.On("Identity").Return("v1/ConfigMap/error-cm")
		resource.On("Object").Return(resourceObject, nil)
		resource.On("Mutate").Return(fmt.Errorf("mutation failed"))

		// When
		_, err := createOrUpdateResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutation failed")

		resource.AssertExpectations(t)
	})
}

func TestMutateResource(t *testing.T) {
	var (
		scheme    = setupScheme()
		namespace = "test-namespace"
		owner     = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-owner",
				Namespace: namespace,
			},
		}
	)

	t.Run("should call Mutate for new objects", func(t *testing.T) {
		// Given
		resource := &MockResource{}
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-mutate",
				Namespace: namespace,
			},
		}
		resource.On("Mutate").Return(nil)

		// When
		err := mutateResource(resource, resourceObject, owner, scheme)

		// Then
		require.NoError(t, err)
		assert.Len(t, resourceObject.OwnerReferences, 1)
		assert.Equal(t, owner.Name, resourceObject.OwnerReferences[0].Name)
		resource.AssertExpectations(t)
	})

	t.Run("should call Mutate only for existing objects", func(t *testing.T) {
		// Given
		resource := &MockResource{}
		now := metav1.Now()
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-mutate-existing",
				Namespace:         namespace,
				CreationTimestamp: now,
			},
		}
		resource.On("Mutate").Return(nil)

		// When
		err := mutateResource(resource, resourceObject, owner, scheme)

		// Then
		require.NoError(t, err)
		resource.AssertExpectations(t)
	})
}
