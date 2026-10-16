package component

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
		err = deleteResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource}})

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
		err := deleteResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource}})

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
		err = deleteResources(ctx, reconcileContext, []reconcileEntry{{Resource: resource1}, {Resource: resource2}})

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
		err := deleteResources(ctx, recCtx, []reconcileEntry{{Resource: resource1}, {Resource: resource2}})

		// Then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete resource v1/ConfigMap/test-cm-1: forbidden")

		mockClient.AssertExpectations(t)
		resource1.AssertExpectations(t)
		resource2.AssertExpectations(t)
	})

	t.Run("should record a deletion event carrying the deleted object and reason", func(t *testing.T) {
		// Given
		resourceObject := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-event",
				Namespace: namespace,
			},
		}
		require.NoError(t, fakeClient.Create(ctx, resourceObject.DeepCopy()))

		resource := &MockResource{}
		resource.On("Object").Return(resourceObject, nil)
		resource.On("Identity").Return("v1/ConfigMap/test-cm-event")

		recCtx := setupReconcileContext(scheme, owner, fakeClient)

		// When
		err := deleteResources(ctx, recCtx, []reconcileEntry{{Resource: resource}}, withDeletionReason("suspension"))

		// Then
		require.NoError(t, err)

		recorder, ok := recCtx.EventRecorder.(*spyRecorder)
		require.True(t, ok)

		deletions := recorder.recordedWithReason("ResourceDeleted")
		require.Len(t, deletions, 1)
		assert.Equal(t, owner, deletions[0].regarding, "event is recorded on the owner")
		assert.Equal(t, "test-cm-event", deletions[0].related.(client.Object).GetName(),
			"deleted resource is attached as the related object")
		assert.Equal(t, corev1.EventTypeNormal, deletions[0].eventType)
		assert.Equal(t, "Delete", deletions[0].action)
		assert.Equal(t, "Resource v1/ConfigMap/test-cm-event deleted due to suspension", deletions[0].note)
	})

	t.Run("should record no event when the resource is already gone", func(t *testing.T) {
		// Given
		resource := &MockResource{}
		resource.On("Object").Return(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm-absent", Namespace: namespace},
		}, nil)
		resource.On("Identity").Return("v1/ConfigMap/test-cm-absent")

		recCtx := setupReconcileContext(scheme, owner, fakeClient)

		// When
		err := deleteResources(ctx, recCtx, []reconcileEntry{{Resource: resource}})

		// Then
		require.NoError(t, err)

		recorder, ok := recCtx.EventRecorder.(*spyRecorder)
		require.True(t, ok)
		assert.Empty(t, recorder.recorded())
	})
}

// claimBeforeDelete returns interceptor funcs that hand another owner the
// controller reference of the named object at the moment it is about to be
// deleted, reproducing an owner claiming the object between the check that
// observed it as safe and the delete.
func claimBeforeDelete(t *testing.T, name string) interceptor.Funcs {
	t.Helper()
	return interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if obj.GetName() == name {
				live := obj.DeepCopyObject().(client.Object)
				require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(obj), live))
				live.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: GroupVersion.String(), Kind: "MockOperatorCRD", Name: "other", UID: "other-uid",
					Controller: ptr.To(true),
				}})
				require.NoError(t, c.Update(ctx, live))
			}
			return c.Delete(ctx, obj, opts...)
		},
	}
}

func TestDeleteResources_BlockOnForeignController(t *testing.T) {
	ctx := t.Context()
	scheme := setupScheme()
	newOwner := func() *MockOperatorCRD {
		return &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default", UID: "this-uid"}}
	}
	newResource := func(name string) (*MockResource, *corev1.ConfigMap) {
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
		res := &MockResource{}
		res.On("Object").Return(obj, nil)
		res.On("Identity").Return("ConfigMap/" + name)
		return res, obj
	}
	guarded := func(res Resource) []reconcileEntry {
		return []reconcileEntry{{Resource: res, Options: resourceOptions{BlockOnForeignController: true}}}
	}

	t.Run("treats an absent object as deleted without calling Delete", func(t *testing.T) {
		owner := newOwner()
		res, _ := newResource("absent")
		deletes := 0
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				deletes++
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()

		require.NoError(t, deleteResources(ctx, setupReconcileContext(scheme, owner, cli), guarded(res)))
		assert.Equal(t, 0, deletes)
	})

	t.Run("deletes an object it observed as safe", func(t *testing.T) {
		owner := newOwner()
		res, obj := newResource("safe")
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).Build()

		require.NoError(t, deleteResources(ctx, setupReconcileContext(scheme, owner, cli), guarded(res)))
		assert.True(t, apierrors.IsNotFound(cli.Get(ctx, client.ObjectKeyFromObject(obj), &corev1.ConfigMap{})))
	})

	t.Run("does not delete an object another owner claims after it was observed as safe", func(t *testing.T) {
		owner := newOwner()
		res, obj := newResource("claimed")
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).
			WithInterceptorFuncs(claimBeforeDelete(t, "claimed")).Build()

		err := deleteResources(ctx, setupReconcileContext(scheme, owner, cli), guarded(res))
		require.Error(t, err, "a delete that lost the race must be reported so the next reconcile rechecks")
		got := &corev1.ConfigMap{}
		require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(obj), got))
		assert.Equal(t, "other-uid", string(got.OwnerReferences[0].UID))
	})
}

func TestDeleteResources_DeleteOnlyIfOwned(t *testing.T) {
	ctx := t.Context()
	scheme := setupScheme()
	newOwner := func() *MockOperatorCRD {
		return &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default", UID: "this-uid"}}
	}
	ownerRef := func(name string, uid string) metav1.OwnerReference {
		return metav1.OwnerReference{APIVersion: GroupVersion.String(), Kind: "MockOperatorCRD", Name: name, UID: types.UID(uid)}
	}
	newResource := func(name string, refs ...metav1.OwnerReference) (*MockResource, *corev1.ConfigMap) {
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", OwnerReferences: refs}}
		res := &MockResource{}
		res.On("Object").Return(obj, nil)
		res.On("Identity").Return("ConfigMap/" + name)
		return res, obj
	}
	ownedOnly := func(res Resource) []reconcileEntry {
		return []reconcileEntry{{Resource: res, Options: resourceOptions{Delete: true, DeleteOnlyIfOwned: true}}}
	}
	countDeletes := func(deletes *int) interceptor.Funcs {
		return interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				*deletes++
				return c.Delete(ctx, obj, opts...)
			},
		}
	}

	t.Run("deletes an object that carries the owner's reference and records the deletion", func(t *testing.T) {
		owner := newOwner()
		res, obj := newResource("owned", ownerRef("owner", "this-uid"))
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).Build()
		rec := setupReconcileContext(scheme, owner, cli)

		require.NoError(t, deleteResources(ctx, rec, ownedOnly(res)))

		assert.True(t, apierrors.IsNotFound(cli.Get(ctx, client.ObjectKeyFromObject(obj), &corev1.ConfigMap{})))
		recorder, ok := rec.EventRecorder.(*spyRecorder)
		require.True(t, ok)
		assert.Len(t, recorder.recordedWithReason("ResourceDeleted"), 1)
	})

	t.Run("deletes an object whose owner reference is not the controller reference", func(t *testing.T) {
		owner := newOwner()
		controller := ownerRef("other", "other-uid")
		controller.Controller = ptr.To(true)
		res, obj := newResource("co-owned", controller, ownerRef("owner", "this-uid"))
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).Build()

		require.NoError(t, deleteResources(ctx, setupReconcileContext(scheme, owner, cli), ownedOnly(res)))

		assert.True(t, apierrors.IsNotFound(cli.Get(ctx, client.ObjectKeyFromObject(obj), &corev1.ConfigMap{})))
	})

	t.Run("deletes with the observed UID and resourceVersion as preconditions", func(t *testing.T) {
		owner := newOwner()
		res, obj := newResource("preconditions", ownerRef("owner", "this-uid"))
		var preconditions *metav1.Preconditions
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					deleteOpts := &client.DeleteOptions{}
					deleteOpts.ApplyOptions(opts)
					preconditions = deleteOpts.Preconditions
					return c.Delete(ctx, obj, opts...)
				},
			}).Build()
		live := &corev1.ConfigMap{}
		require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(obj), live))

		require.NoError(t, deleteResources(ctx, setupReconcileContext(scheme, owner, cli), ownedOnly(res)))

		require.NotNil(t, preconditions)
		require.NotNil(t, preconditions.UID)
		require.NotNil(t, preconditions.ResourceVersion)
		assert.Equal(t, live.UID, *preconditions.UID)
		assert.Equal(t, live.ResourceVersion, *preconditions.ResourceVersion)
	})

	t.Run("keeps an object without the owner's reference and writes nothing", func(t *testing.T) {
		owner := newOwner()
		res, obj := newResource("user-created")
		deletes := 0
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).
			WithInterceptorFuncs(countDeletes(&deletes)).Build()
		before := &corev1.ConfigMap{}
		require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(obj), before))
		rec := setupReconcileContext(scheme, owner, cli)

		require.NoError(t, deleteResources(ctx, rec, ownedOnly(res)))

		assert.Equal(t, 0, deletes)
		after := &corev1.ConfigMap{}
		require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(obj), after))
		assert.Equal(t, before.ResourceVersion, after.ResourceVersion)
		recorder, ok := rec.EventRecorder.(*spyRecorder)
		require.True(t, ok)
		assert.Empty(t, recorder.recorded(), "a kept object must not be reported as deleted")
	})

	t.Run("keeps an object that only another owner references", func(t *testing.T) {
		owner := newOwner()
		res, obj := newResource("other-owned", ownerRef("other", "other-uid"))
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).Build()

		require.NoError(t, deleteResources(ctx, setupReconcileContext(scheme, owner, cli), ownedOnly(res)))

		assert.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(obj), &corev1.ConfigMap{}))
	})

	t.Run("treats an absent object as deleted without calling Delete", func(t *testing.T) {
		owner := newOwner()
		res, _ := newResource("absent")
		deletes := 0
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).
			WithInterceptorFuncs(countDeletes(&deletes)).Build()

		require.NoError(t, deleteResources(ctx, setupReconcileContext(scheme, owner, cli), ownedOnly(res)))

		assert.Equal(t, 0, deletes)
	})

	t.Run("keeps an object that changed after it was read without failing", func(t *testing.T) {
		owner := newOwner()
		res, obj := newResource("changed", ownerRef("owner", "this-uid"))
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).
			WithInterceptorFuncs(disownBeforeDelete(t, "changed")).Build()
		rec := setupReconcileContext(scheme, owner, cli)

		require.NoError(t, deleteResources(ctx, rec, ownedOnly(res)))

		got := &corev1.ConfigMap{}
		require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(obj), got))
		assert.Empty(t, got.OwnerReferences)
		recorder, ok := rec.EventRecorder.(*spyRecorder)
		require.True(t, ok)
		assert.Empty(t, recorder.recorded())
	})

	t.Run("still reports a delete failure that is not a precondition conflict", func(t *testing.T) {
		owner := newOwner()
		res, obj := newResource("forbidden", ownerRef("owner", "this-uid"))
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
					return apierrors.NewForbidden(corev1.Resource("configmaps"), "forbidden", errors.New("denied"))
				},
			}).Build()

		err := deleteResources(ctx, setupReconcileContext(scheme, owner, cli), ownedOnly(res))

		require.Error(t, err)
		assert.True(t, apierrors.IsForbidden(err))
	})

	t.Run("keeps an object another owner controls when combined with BlockOnForeignController", func(t *testing.T) {
		owner := newOwner()
		controller := ownerRef("other", "other-uid")
		controller.Controller = ptr.To(true)
		res, obj := newResource("foreign", controller, ownerRef("owner", "this-uid"))
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).Build()
		entries := []reconcileEntry{{
			Resource: res,
			Options:  resourceOptions{Delete: true, DeleteOnlyIfOwned: true, BlockOnForeignController: true},
		}}

		require.NoError(t, deleteResources(ctx, setupReconcileContext(scheme, owner, cli), entries))

		assert.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(obj), &corev1.ConfigMap{}))
	})
}

// disownBeforeDelete returns interceptor funcs that remove every owner
// reference from the named object at the moment it is about to be deleted,
// reproducing a change between the read that found the object owned and the
// delete.
func disownBeforeDelete(t *testing.T, name string) interceptor.Funcs {
	t.Helper()
	return interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if obj.GetName() == name {
				live := obj.DeepCopyObject().(client.Object)
				require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(obj), live))
				live.SetOwnerReferences(nil)
				require.NoError(t, c.Update(ctx, live))
			}
			return c.Delete(ctx, obj, opts...)
		},
	}
}
