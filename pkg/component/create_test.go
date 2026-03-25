package component

import (
	"fmt"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// createTestRESTMapper returns a mapper with ConfigMap (namespace-scoped) and
// MockOperatorCRD (namespace-scoped) registered.
func createTestRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		corev1.SchemeGroupVersion,
		GroupVersion,
	})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(GroupVersion.WithKind("MockOperatorCRD"), meta.RESTScopeNamespace)
	return mapper
}

func TestApplyResources(t *testing.T) {
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
		resource.On("Mutate", mock.Anything).Return(nil)

		// When
		results, err := applyResources(ctx, reconcileContext, []Resource{resource}, "test-component", createTestRESTMapper())

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
		resource1.On("Mutate", mock.Anything).Return(nil)

		resourceObject2 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-2",
				Namespace: namespace,
			},
		}
		resource2 := &MockResource{}
		resource2.On("Object").Return(resourceObject2, nil)
		resource2.On("Mutate", mock.Anything).Return(nil)

		// When
		results, err := applyResources(ctx, reconcileContext, []Resource{resource1, resource2}, "test-component", createTestRESTMapper())

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
		regularResource.On("Mutate", mock.Anything).Return(nil)

		aliveResourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-alive-mixed",
				Namespace: namespace,
			},
		}
		aliveResource := &MockAliveResource{}
		aliveResource.On("Object").Return(aliveResourceObject, nil)
		aliveResource.On("Mutate", mock.Anything).Return(nil)
		aliveResource.On("ConvergingStatus", mock.Anything).Return(concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusHealthy,
			Reason: "Ready",
		}, nil)

		// When
		results, err := applyResources(ctx, reconcileContext, []Resource{regularResource, aliveResource}, "test-component", createTestRESTMapper())

		// Then
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, aliveResource, results[0].Resource)
		assert.Equal(t, convergingStatusAliveHealthy, results[0].Status.Status)

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
		resource1.On("Mutate", mock.Anything).Return(nil)

		resource2 := &MockResource{}
		resource2.On("Identity").Return("v1/ConfigMap/failed-resource")
		resource2.On("Object").Return(nil, fmt.Errorf("resource 2 error"))

		resource3 := &MockResource{} // Should not be processed

		// When
		results, err := applyResources(ctx, reconcileContext, []Resource{resource1, resource2, resource3}, "test-component", createTestRESTMapper())

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
		resource.On("Mutate", mock.Anything).Run(func(args mock.Arguments) {
			obj := args.Get(0).(*corev1.ConfigMap)
			obj.Data["foo"] = "baz"
		}).Return(nil)

		// When
		results, err := applyResources(ctx, reconcileContext, []Resource{resource}, "test-component", createTestRESTMapper())

		// Then
		require.NoError(t, err)
		assert.Empty(t, results)

		updatedConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, updatedConfigMap)
		require.NoError(t, err)
		assert.Equal(t, "baz", updatedConfigMap.Data["foo"])
		resource.AssertExpectations(t)
	})

	for _, tc := range []struct {
		name           string
		resource       Resource
		expectedStatus convergingStatus
	}{
		{
			name: "should handle Alive resources",
			resource: func() Resource {
				r := &MockAliveResource{}
				r.On("Object").Return(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "test-alive", Namespace: namespace},
				}, nil)
				r.On("Mutate", mock.Anything).Return(nil)
				r.On("ConvergingStatus", mock.Anything).Return(concepts.AliveStatusWithReason{
					Status: concepts.AliveConvergingStatusHealthy, Reason: "Ready",
				}, nil)
				return r
			}(),
			expectedStatus: convergingStatusAliveHealthy,
		},
		{
			name: "should handle Operational resources",
			resource: func() Resource {
				r := &MockOperationalResource{}
				r.On("Object").Return(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "test-operational", Namespace: namespace},
				}, nil)
				r.On("Mutate", mock.Anything).Return(nil)
				r.On("ConvergingStatus", mock.Anything).Return(concepts.OperationalStatusWithReason{
					Status: concepts.OperationalStatusOperational, Reason: "Operational",
				}, nil)
				return r
			}(),
			expectedStatus: convergingStatusOperationalOperational,
		},
		{
			name: "should handle Completable resources",
			resource: func() Resource {
				r := &MockCompletableResource{}
				r.On("Object").Return(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "test-completable", Namespace: namespace},
				}, nil)
				r.On("Mutate", mock.Anything).Return(nil)
				r.On("ConvergingStatus", mock.Anything).Return(concepts.CompletionStatusWithReason{
					Status: concepts.CompletionStatusCompleted, Reason: "Completed",
				}, nil)
				return r
			}(),
			expectedStatus: convergingStatusCompletableCompleted,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, err := applyResources(ctx, reconcileContext, []Resource{tc.resource}, "test-component", createTestRESTMapper())

			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, tc.resource, results[0].Resource)
			assert.Equal(t, tc.expectedStatus, results[0].Status.Status)
		})
	}

	t.Run("should return error if resource.Object() fails", func(t *testing.T) {
		// Given
		resource := &MockResource{}
		resource.On("Identity").Return("v1/ConfigMap/failed-resource")
		resource.On("Object").Return(nil, fmt.Errorf("object error"))

		// When
		_, err := applyResources(ctx, reconcileContext, []Resource{resource}, "test-component", createTestRESTMapper())

		// Then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to retrieve object")
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
		resource.On("Mutate", mock.Anything).Return(fmt.Errorf("mutation failed"))

		// When
		_, err := applyResources(ctx, reconcileContext, []Resource{resource}, "test-component", createTestRESTMapper())

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
		mapper = createTestRESTMapper()
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
		resource.On("Mutate", mock.Anything).Return(nil)
		resource.On("Identity").Maybe().Return("v1/ConfigMap/test-namespace/test-mutate")

		// When
		_, err := mutateResource(resource, resourceObject, owner, scheme, mapper)

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
		resource.On("Mutate", mock.Anything).Return(nil)
		resource.On("Identity").Maybe().Return("v1/ConfigMap/test-namespace/test-mutate-existing")

		// When
		_, err := mutateResource(resource, resourceObject, owner, scheme, mapper)

		// Then
		require.NoError(t, err)
		resource.AssertExpectations(t)
	})

	t.Run("should skip owner reference for cluster-scoped resource with namespaced owner", func(t *testing.T) {
		// Given
		clusterScopedMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
			{Group: "rbac.authorization.k8s.io", Version: "v1"},
			corev1.SchemeGroupVersion,
			GroupVersion,
		})
		clusterScopedMapper.Add(
			schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
			meta.RESTScopeRoot,
		)
		clusterScopedMapper.Add(GroupVersion.WithKind("MockOperatorCRD"), meta.RESTScopeNamespace)

		resource := &MockResource{}
		resourceObject := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster-role",
			},
		}
		resource.On("Mutate", mock.Anything).Return(nil)
		resource.On("Identity").Maybe().Return("rbac.authorization.k8s.io/v1/ClusterRole/test-cluster-role")

		// When
		skipped, err := mutateResource(resource, resourceObject, owner, scheme, clusterScopedMapper)

		// Then
		require.NoError(t, err)
		assert.True(t, skipped, "owner reference should be skipped for cluster-scoped resource with namespaced owner")
		assert.Empty(t, resourceObject.OwnerReferences, "no owner reference should be set")
		resource.AssertExpectations(t)
	})
}

func TestApplyResources_ClusterScopedResource(t *testing.T) {
	var (
		scheme    = setupScheme()
		namespace = "test-namespace"
		owner     = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-owner",
				Namespace:  namespace,
				Generation: 1,
			},
		}
	)

	clusterScopedMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
		corev1.SchemeGroupVersion,
		GroupVersion,
	})
	clusterScopedMapper.Add(
		schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
		meta.RESTScopeRoot,
	)
	clusterScopedMapper.Add(GroupVersion.WithKind("MockOperatorCRD"), meta.RESTScopeNamespace)

	t.Run("should create cluster-scoped resource without owner reference", func(t *testing.T) {
		// Given
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).WithStatusSubresource(owner).Build()
		reconcileContext := setupReconcileContext(scheme, owner, fakeClient)
		ctx := t.Context()

		resourceObject := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster-role-create",
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(resourceObject, nil)
		resource.On("Mutate", mock.Anything).Return(nil)
		resource.On("Identity").Maybe().Return("rbac.authorization.k8s.io/v1/ClusterRole/test-cluster-role-create")

		// When
		results, err := applyResources(ctx, reconcileContext, []Resource{resource}, "test-component", clusterScopedMapper)

		// Then
		require.NoError(t, err)
		assert.Empty(t, results)

		createdRole := &rbacv1.ClusterRole{}
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-cluster-role-create"}, createdRole)
		require.NoError(t, err)
		assert.Empty(t, createdRole.OwnerReferences, "cluster-scoped resource should have no owner references")
		resource.AssertExpectations(t)
	})
}
