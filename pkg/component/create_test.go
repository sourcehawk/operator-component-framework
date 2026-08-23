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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
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
		results, err := applyResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource}}, "test-component", createTestRESTMapper())

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
		results, err := applyResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource1}, {Resource: resource2}}, "test-component", createTestRESTMapper())

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
		results, err := applyResources(ctx, reconcileContext, []reconcileEntry{{Resource: regularResource}, {Resource: aliveResource}}, "test-component", createTestRESTMapper())

		// Then
		require.NoError(t, err)
		require.Len(t, results, 1)
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
		results, err := applyResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource1}, {Resource: resource2}, {Resource: resource3}}, "test-component", createTestRESTMapper())

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
		results, err := applyResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource}}, "test-component", createTestRESTMapper())

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
			results, err := applyResources(ctx, reconcileContext, []reconcileEntry{{Resource: tc.resource}}, "test-component", createTestRESTMapper())

			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, tc.expectedStatus, results[0].Status.Status)
		})
	}

	t.Run("should return error if resource.Object() fails", func(t *testing.T) {
		// Given
		resource := &MockResource{}
		resource.On("Identity").Return("v1/ConfigMap/failed-resource")
		resource.On("Object").Return(nil, fmt.Errorf("object error"))

		// When
		_, err := applyResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource}}, "test-component", createTestRESTMapper())

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
		_, err := applyResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource}}, "test-component", createTestRESTMapper())

		// Then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutation failed")

		resource.AssertExpectations(t)
	})
}

// operationRecordingResource is an Alive resource that builds its desired object
// from scratch on every Object() call, mirroring operators that rebuild their
// components on each reconcile. It records the converging operation passed to
// ConvergingStatus so tests can assert how each apply was classified.
type operationRecordingResource struct {
	build      func() *corev1.ConfigMap
	mutate     func(*corev1.ConfigMap)
	operations []concepts.ConvergingOperation
}

func (r *operationRecordingResource) Identity() string { return "ConfigMap/operation-recording" }

func (r *operationRecordingResource) Object() (client.Object, error) { return r.build(), nil }

func (r *operationRecordingResource) Mutate(obj client.Object) error {
	if r.mutate != nil {
		r.mutate(obj.(*corev1.ConfigMap))
	}
	return nil
}

func (r *operationRecordingResource) ConvergingStatus(
	op concepts.ConvergingOperation,
) (concepts.AliveStatusWithReason, error) {
	r.operations = append(r.operations, op)
	return concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusHealthy, Reason: "ok"}, nil
}

func (r *operationRecordingResource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}, nil
}

// objectOperationRecordingResource is the counterpart of
// operationRecordingResource for any client.Object type.
type objectOperationRecordingResource struct {
	build      func() client.Object
	operations []concepts.ConvergingOperation
	// applied is the object handed to Mutate. The framework decodes the apply
	// response into that same object, so after a reconcile it holds what the
	// server returned.
	applied client.Object
}

func (r *objectOperationRecordingResource) Identity() string {
	return "operation-recording-object"
}

func (r *objectOperationRecordingResource) Object() (client.Object, error) {
	return r.build(), nil
}

func (r *objectOperationRecordingResource) Mutate(obj client.Object) error {
	r.applied = obj
	return nil
}

func (r *objectOperationRecordingResource) ConvergingStatus(
	op concepts.ConvergingOperation,
) (concepts.AliveStatusWithReason, error) {
	r.operations = append(r.operations, op)
	return concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusHealthy, Reason: "ok"}, nil
}

func (r *objectOperationRecordingResource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}, nil
}

func TestApplyResource_ConvergingOperation(t *testing.T) {
	const namespace = "test-namespace"

	newEnv := func(t *testing.T) (ReconcileContext, client.Client) {
		t.Helper()
		scheme := setupScheme()
		owner := &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: namespace, UID: "owner-uid"},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).WithStatusSubresource(owner).Build()
		return setupReconcileContext(scheme, owner, fakeClient), fakeClient
	}

	buildConfigMap := func(data map[string]string) func() *corev1.ConfigMap {
		return func() *corev1.ConfigMap {
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rebuilt-cm", Namespace: namespace}}
			for k, v := range data {
				if cm.Data == nil {
					cm.Data = map[string]string{}
				}
				cm.Data[k] = v
			}
			return cm
		}
	}

	apply := func(t *testing.T, rec ReconcileContext, res Resource) {
		t.Helper()
		_, err := applyResources(t.Context(), rec, []reconcileEntry{{Resource: res}}, "test-component", createTestRESTMapper())
		require.NoError(t, err)
	}

	t.Run("reports None when a rebuilt desired object matches what is already applied", func(t *testing.T) {
		rec, _ := newEnv(t)
		res := &operationRecordingResource{build: buildConfigMap(map[string]string{"foo": "bar"})}

		apply(t, rec, res)
		apply(t, rec, res)
		apply(t, rec, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationNone,
			concepts.ConvergingOperationNone,
		}, res.operations)
		assert.Empty(t, rec.EventRecorder.(*spyRecorder).recordedWithReason("UpdatedConfigMap"))
	})

	t.Run("reports None when feature mutations are re-applied to a rebuilt desired object", func(t *testing.T) {
		rec, _ := newEnv(t)
		res := &operationRecordingResource{
			build:  buildConfigMap(map[string]string{"foo": "bar"}),
			mutate: func(cm *corev1.ConfigMap) { cm.Data["feature"] = "enabled" },
		}

		apply(t, rec, res)
		apply(t, rec, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationNone,
		}, res.operations)
	})

	t.Run("reports Updated when the rebuilt desired object changes", func(t *testing.T) {
		rec, _ := newEnv(t)
		res := &operationRecordingResource{build: buildConfigMap(map[string]string{"foo": "bar"})}
		apply(t, rec, res)

		res.build = buildConfigMap(map[string]string{"foo": "baz"})
		apply(t, rec, res)
		apply(t, rec, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationUpdated,
			concepts.ConvergingOperationNone,
		}, res.operations)
		assert.Len(t, rec.EventRecorder.(*spyRecorder).recordedWithReason("UpdatedConfigMap"), 1)
	})

	t.Run("reports Updated when a feature mutation starts changing the applied object", func(t *testing.T) {
		rec, _ := newEnv(t)
		res := &operationRecordingResource{build: buildConfigMap(map[string]string{"foo": "bar"})}
		apply(t, rec, res)

		res.mutate = func(cm *corev1.ConfigMap) { cm.Data["feature"] = "enabled" }
		apply(t, rec, res)
		apply(t, rec, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationUpdated,
			concepts.ConvergingOperationNone,
		}, res.operations)
	})

	t.Run("reports Updated when adopting an object that already exists without the owner reference", func(t *testing.T) {
		rec, c := newEnv(t)
		preexisting := buildConfigMap(map[string]string{"foo": "bar"})()
		require.NoError(t, c.Create(t.Context(), preexisting))

		res := &operationRecordingResource{build: buildConfigMap(map[string]string{"foo": "bar"})}
		apply(t, rec, res)
		apply(t, rec, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationUpdated,
			concepts.ConvergingOperationNone,
		}, res.operations)
	})

	t.Run("classifies unstructured objects the same way", func(t *testing.T) {
		rec, _ := newEnv(t)
		data := map[string]any{"foo": "bar"}
		build := func() client.Object {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
			u.SetName("rebuilt-unstructured")
			u.SetNamespace(namespace)
			require.NoError(t, unstructured.SetNestedField(u.Object, runtime.DeepCopyJSON(data), "data"))
			return u
		}
		res := &objectOperationRecordingResource{build: build}

		apply(t, rec, res)
		apply(t, rec, res)
		data["foo"] = "baz"
		apply(t, rec, res)
		apply(t, rec, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationNone,
			concepts.ConvergingOperationUpdated,
			concepts.ConvergingOperationNone,
		}, res.operations)
	})

	t.Run("reports None when a persistent desired object is re-applied unchanged", func(t *testing.T) {
		rec, _ := newEnv(t)
		// Same pointer on every reconcile: the SSA response is decoded back into it,
		// which is how BaseResource.DesiredObject behaves across reconciles.
		persistent := buildConfigMap(map[string]string{"foo": "bar"})()
		res := &operationRecordingResource{build: func() *corev1.ConfigMap { return persistent }}

		apply(t, rec, res)
		apply(t, rec, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationNone,
		}, res.operations)
	})
}

// typeMetaClearingResource models a Mutate that assigns the whole struct, a
// realistic way to copy desired state onto the current object. That drops the
// TypeMeta the framework set, and Server-Side Apply requires it.
type typeMetaClearingResource struct {
	namespace string
}

func (r *typeMetaClearingResource) Identity() string {
	return "v1/ConfigMap/" + r.namespace + "/typemeta-cm"
}

func (r *typeMetaClearingResource) Object() (client.Object, error) {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "typemeta-cm", Namespace: r.namespace},
	}, nil
}

func (r *typeMetaClearingResource) Mutate(obj client.Object) error {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return fmt.Errorf("expected *corev1.ConfigMap, got %T", obj)
	}
	*cm = corev1.ConfigMap{ObjectMeta: cm.ObjectMeta, Data: map[string]string{"foo": "bar"}}
	return nil
}

func TestApplyResource_ReassertsGVKAfterMutate(t *testing.T) {
	const namespace = "test-namespace"
	scheme := setupScheme()
	owner := &MockOperatorCRD{
		ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: namespace, UID: "owner-uid"},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).WithObjects(owner).WithStatusSubresource(owner).Build()
	rec := setupReconcileContext(scheme, owner, fakeClient)

	res := &typeMetaClearingResource{namespace: namespace}
	_, err := applyResources(
		t.Context(), rec, []reconcileEntry{{Resource: res}}, "test-component", createTestRESTMapper(),
	)
	require.NoError(t, err)

	applied := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(
		t.Context(), client.ObjectKey{Name: "typemeta-cm", Namespace: namespace}, applied,
	))
	assert.Equal(t, map[string]string{"foo": "bar"}, applied.Data)
}

func TestNewEmptyObjectLike(t *testing.T) {
	t.Run("returns a zeroed typed object of the same type", func(t *testing.T) {
		src := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "src", Namespace: "ns"},
			Data:       map[string]string{"k": "v"},
		}
		empty, err := newEmptyObjectLike(src)
		require.NoError(t, err)
		cm, ok := empty.(*corev1.ConfigMap)
		require.True(t, ok)
		assert.Empty(t, cm.Name)
		assert.Nil(t, cm.Data)
	})

	t.Run("carries the GVK for unstructured objects", func(t *testing.T) {
		src := &unstructured.Unstructured{}
		src.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
		src.SetName("src")
		empty, err := newEmptyObjectLike(src)
		require.NoError(t, err)
		u, ok := empty.(*unstructured.Unstructured)
		require.True(t, ok)
		assert.Equal(t, corev1.SchemeGroupVersion.WithKind("ConfigMap"), u.GroupVersionKind())
		assert.Empty(t, u.GetName())
	})

	t.Run("rejects a nil interface instead of panicking", func(t *testing.T) {
		_, err := newEmptyObjectLike(nil)
		require.Error(t, err)
	})

	t.Run("rejects a typed nil pointer instead of panicking", func(t *testing.T) {
		var cm *corev1.ConfigMap
		_, err := newEmptyObjectLike(cm)
		require.Error(t, err)
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
		_, err := mutateResource(resource, resourceObject, owner, scheme, mapper, false)

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
		_, err := mutateResource(resource, resourceObject, owner, scheme, mapper, false)

		// Then
		require.NoError(t, err)
		resource.AssertExpectations(t)
	})

	t.Run("should not set owner reference when skipOwnerRef is true", func(t *testing.T) {
		// Given
		resource := &MockResource{}
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-unowned",
				Namespace: namespace,
			},
		}
		resource.On("Mutate", mock.Anything).Return(nil)
		resource.On("Identity").Maybe().Return("v1/ConfigMap/test-namespace/test-unowned")

		// When
		skipped, err := mutateResource(resource, resourceObject, owner, scheme, mapper, true)

		// Then
		require.NoError(t, err)
		assert.False(t, skipped, "intentional Unowned skip must not be reported as a scope-incompatibility skip")
		assert.Empty(t, resourceObject.OwnerReferences, "no owner reference should be set for an Unowned resource")
		resource.AssertExpectations(t)
	})

	t.Run("should clear a previously-cached owner reference when skipOwnerRef is true", func(t *testing.T) {
		// The DesiredObject pointer retains the owner ref written back by the server
		// after the first reconcile (Patch writes into the same pointer). If Unowned()
		// is added later, that cached ref must be cleared so the SSA patch does not
		// re-apply it.
		localOwner := &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-owner-cached",
				Namespace: namespace,
				UID:       "owner-uid-cached",
			},
		}
		resource := &MockResource{}
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-unowned-cached",
				Namespace: namespace,
				OwnerReferences: []metav1.OwnerReference{
					{APIVersion: "test/v1", Kind: "MockOperatorCRD", Name: localOwner.Name, UID: localOwner.UID, Controller: ptr.To(true)},
				},
			},
		}
		resource.On("Mutate", mock.Anything).Return(nil)
		resource.On("Identity").Maybe().Return("v1/ConfigMap/test-namespace/test-unowned-cached")

		_, err := mutateResource(resource, resourceObject, localOwner, scheme, mapper, true)

		require.NoError(t, err)
		assert.Empty(t, resourceObject.OwnerReferences, "cached owner reference must be cleared for an Unowned resource")
		resource.AssertExpectations(t)
	})

	t.Run("should preserve ownerReferences to other objects when skipOwnerRef is true", func(t *testing.T) {
		// Mutate() may add owner references to objects other than the component's owner
		// CR (e.g., ownership by a different controller). Those must be preserved; only
		// the reference pointing to the component owner should be suppressed.
		resource := &MockResource{}
		otherRef := metav1.OwnerReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       "other-owner",
			UID:        "other-owner-uid",
		}
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-unowned-other-refs",
				Namespace: namespace,
			},
		}
		resource.On("Mutate", mock.Anything).Run(func(args mock.Arguments) {
			obj := args.Get(0).(client.Object)
			obj.SetOwnerReferences([]metav1.OwnerReference{otherRef})
		}).Return(nil)
		resource.On("Identity").Maybe().Return("v1/ConfigMap/test-namespace/test-unowned-other-refs")

		_, err := mutateResource(resource, resourceObject, owner, scheme, mapper, true)

		require.NoError(t, err)
		assert.Equal(t, []metav1.OwnerReference{otherRef}, resourceObject.OwnerReferences,
			"ownerReferences to other objects set in Mutate must be preserved")
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
		skipped, err := mutateResource(resource, resourceObject, owner, scheme, clusterScopedMapper, false)

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
		results, err := applyResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource}}, "test-component", clusterScopedMapper)

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

func TestReconcileResources_Unowned(t *testing.T) {
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
		fakeClient       = fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).WithStatusSubresource(owner).Build()
		reconcileContext = setupReconcileContext(scheme, owner, fakeClient)
		mapper           = createTestRESTMapper()
		ctx              = t.Context()
	)

	t.Run("does not set owner reference on an Unowned resource", func(t *testing.T) {
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-unowned-cm",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(resourceObject, nil)
		resource.On("Mutate", mock.Anything).Return(nil)

		entry := reconcileEntry{
			Resource: resource,
			Options:  resourceOptions{Unowned: true, ParticipationMode: ParticipationModeRequired},
		}

		_, err := reconcileResources(ctx, reconcileContext, []reconcileEntry{entry}, "comp", mapper)
		require.NoError(t, err)

		created := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-unowned-cm", Namespace: namespace}, created)
		require.NoError(t, err)
		assert.Empty(t, created.OwnerReferences, "Unowned resource must not have an owner reference")
		resource.AssertExpectations(t)
	})
}

func TestReconcileResources_BlockOnAbsence(t *testing.T) {
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
		mapper           = createTestRESTMapper()
		ctx              = t.Context()
	)

	t.Run("a missing read-only resource that opted into BlockOnAbsence reports guard-blocked instead of erroring", func(t *testing.T) {
		// Given a read-only resource pointed at a Secret that does not exist
		// in the cluster, registered with BlockOnAbsence: true.
		missing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "absent-secret",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(missing, nil)
		resource.On("Identity").Return("v1/Secret/absent-secret")

		entry := reconcileEntry{
			Resource: resource,
			Options:  resourceOptions{ReadOnly: true, BlockOnAbsence: true},
		}

		// When
		results, err := reconcileResources(ctx, reconcileContext, []reconcileEntry{entry}, "comp", mapper)

		// Then no error; one guard-blocked result with the resource identity in the reason.
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, convergingStatusGuardBlocked, results[0].Status.Status)
		assert.Contains(t, results[0].Status.Reason, "v1/Secret/absent-secret")
	})

	t.Run("a missing read-only resource without BlockOnAbsence still errors", func(t *testing.T) {
		missing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "absent-secret-strict",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(missing, nil)
		resource.On("Identity").Return("v1/Secret/absent-secret-strict")

		entry := reconcileEntry{
			Resource: resource,
			Options:  resourceOptions{ReadOnly: true},
		}

		_, err := reconcileResources(ctx, reconcileContext, []reconcileEntry{entry}, "comp", mapper)
		require.Error(t, err)
	})

	t.Run("subsequent resources are skipped after a BlockOnAbsence short-circuit", func(t *testing.T) {
		missing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "absent-secret-leader",
				Namespace: namespace,
			},
		}
		leader := &MockResource{}
		leader.On("Object").Return(missing, nil)
		leader.On("Identity").Return("v1/Secret/absent-secret-leader")

		// A second resource that, if reached, would fail the assertion: its
		// Object call is not registered, so an unexpected invocation panics.
		follower := &MockResource{}

		entries := []reconcileEntry{
			{Resource: leader, Options: resourceOptions{ReadOnly: true, BlockOnAbsence: true}},
			{Resource: follower, Options: resourceOptions{ReadOnly: true}},
		}

		results, err := reconcileResources(ctx, reconcileContext, entries, "comp", mapper)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, convergingStatusGuardBlocked, results[0].Status.Status)
		follower.AssertNotCalled(t, "Object")
	})
}

func TestReconcileResources_IgnoreIfAbsent(t *testing.T) {
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
		mapper           = createTestRESTMapper()
		ctx              = t.Context()
	)

	t.Run("a missing read-only resource with IgnoreIfAbsent is silently skipped", func(t *testing.T) {
		missing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "absent-optional-secret",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(missing, nil)
		resource.On("Identity").Return("v1/Secret/absent-optional-secret")

		entry := reconcileEntry{
			Resource: resource,
			Options:  resourceOptions{ReadOnly: true, IgnoreIfAbsent: true},
		}

		results, err := reconcileResources(ctx, reconcileContext, []reconcileEntry{entry}, "comp", mapper)

		require.NoError(t, err)
		assert.Empty(t, results, "absent IgnoreIfAbsent resource must contribute no condition")
	})

	t.Run("subsequent resources still reconcile after an ignored absence", func(t *testing.T) {
		missingLeader := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "absent-optional-leader",
				Namespace: namespace,
			},
		}
		leader := &MockResource{}
		leader.On("Object").Return(missingLeader, nil)
		leader.On("Identity").Return("v1/Secret/absent-optional-leader")

		presentFollower := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "present-follower",
				Namespace: namespace,
			},
		}
		require.NoError(t, fakeClient.Create(ctx, presentFollower))

		follower := &MockResource{}
		follower.On("Object").Return(presentFollower, nil)
		follower.On("Identity").Return("v1/Secret/present-follower")

		entries := []reconcileEntry{
			{Resource: leader, Options: resourceOptions{ReadOnly: true, IgnoreIfAbsent: true}},
			{Resource: follower, Options: resourceOptions{ReadOnly: true}},
		}

		_, err := reconcileResources(ctx, reconcileContext, entries, "comp", mapper)

		require.NoError(t, err)
		follower.AssertCalled(t, "Object")
	})

	t.Run("a missing read-only resource without any absence flag still errors", func(t *testing.T) {
		missing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "absent-strict-default",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(missing, nil)
		resource.On("Identity").Return("v1/Secret/absent-strict-default")

		entry := reconcileEntry{
			Resource: resource,
			Options:  resourceOptions{ReadOnly: true},
		}

		_, err := reconcileResources(ctx, reconcileContext, []reconcileEntry{entry}, "comp", mapper)
		require.Error(t, err)
	})
}
