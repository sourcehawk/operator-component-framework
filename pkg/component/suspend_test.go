package component

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// testRESTMapper returns a RESTMapper with ConfigMap and MockOperatorCRD registered as namespace-scoped.
func testRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		v1.SchemeGroupVersion,
		GroupVersion,
	})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(GroupVersion.WithKind("MockOperatorCRD"), meta.RESTScopeNamespace)
	return mapper
}

func setupTestOwner() *MockOperatorCRD {
	owner := &MockOperatorCRD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner",
			Namespace: "default",
		},
	}
	owner.SetGroupVersionKind(schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "MockOperatorCRD"})
	return owner
}

func setupMockResource(name string, status concepts.SuspensionStatus, reason string, deleteOnSuspend bool) (*MockSuspendableResource, *v1.ConfigMap) {
	res := &MockSuspendableResource{}
	obj := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}

	res.On("Suspend").Return(nil)
	res.On("Object").Return(obj, nil)
	res.On("Identity").Return(name)
	res.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{Status: status, Reason: reason}, nil)
	res.On("DeleteOnSuspend").Return(deleteOnSuspend)
	res.On("Mutate", mock.Anything).Return(nil)

	return res, obj
}

func TestSuspensionResultsSummary(t *testing.T) {
	tests := []struct {
		name     string
		results  suspensionResults
		expected concepts.SuspensionStatusWithReason
	}{
		{
			name:    "empty results",
			results: suspensionResults{},
			expected: concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: "All resources are suspended.",
			},
		},
		{
			name: "all suspended",
			results: suspensionResults{
				{Status: concepts.SuspensionStatusSuspended, Reason: "Suspended 1"},
				{Status: concepts.SuspensionStatusSuspended, Reason: "Suspended 2"},
			},
			expected: concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: "All resources are suspended.",
			},
		},
		{
			name: "mixed statuses, highest priority wins (Pending > Suspending > Suspended)",
			results: suspensionResults{
				{Status: concepts.SuspensionStatusSuspended, Reason: "Suspended"},
				{Status: concepts.SuspensionStatusSuspending, Reason: "Scaling down"},
				{Status: concepts.SuspensionStatusPending, Reason: "Waiting for DB"},
			},
			expected: concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusPending,
				Reason: "Waiting for DB",
			},
		},
		{
			name: "same statuses aggregate reasons",
			results: suspensionResults{
				{Status: concepts.SuspensionStatusSuspending, Reason: "Scaling down"},
				{Status: concepts.SuspensionStatusSuspending, Reason: "Finishing tasks"},
			},
			expected: concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspending,
				Reason: "Scaling down; Finishing tasks",
			},
		},
		{
			name: "highest priority has empty reasons",
			results: suspensionResults{
				{Status: concepts.SuspensionStatusSuspending, Reason: ""},
				{Status: concepts.SuspensionStatusSuspending, Reason: ""},
				{Status: concepts.SuspensionStatusSuspended, Reason: "Done"},
			},
			expected: concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspending,
				Reason: "",
			},
		},
		{
			name: "mixed empty and non-empty reasons for highest priority",
			results: suspensionResults{
				{Status: concepts.SuspensionStatusSuspending, Reason: "Scaling"},
				{Status: concepts.SuspensionStatusSuspending, Reason: ""},
				{Status: concepts.SuspensionStatusSuspending, Reason: "Waiting"},
			},
			expected: concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspending,
				Reason: "Scaling; Waiting",
			},
		},
		{
			name: "unknown status level",
			results: suspensionResults{
				{Status: "Unknown", Reason: "Something"},
			},
			expected: concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: "All resources are suspended.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.results.summary())
		})
	}
}

func TestSuspendResources(t *testing.T) {
	ctx := t.Context()
	scheme := setupScheme()

	t.Run("should suspend all suspendable resources", func(t *testing.T) {
		owner := setupTestOwner()
		r1, obj1 := setupMockResource("r1", concepts.SuspensionStatusSuspended, "OK", false)
		r2, obj2 := setupMockResource("r2", concepts.SuspensionStatusSuspending, "Wait", false)
		nonSuspendable := &MockResource{}

		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj1, obj2).Build()
		rec := setupReconcileContext(scheme, owner, cli)

		resources := []reconcileEntry{{Resource: r1}, {Resource: r2}, {Resource: nonSuspendable}}
		results, err := suspendResources(ctx, rec, resources, "test-component", testRESTMapper())

		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, concepts.SuspensionStatusSuspended, results[0].Status)
		assert.Equal(t, concepts.SuspensionStatusSuspending, results[1].Status)
	})

	t.Run("should return joined errors if any suspension fails", func(t *testing.T) {
		rec := setupReconcileContext(scheme, nil, &MockClient{})
		obj1 := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default"}}
		obj2 := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "r2", Namespace: "default"}}
		r1 := &MockSuspendableResource{}
		r1.On("Object").Return(obj1, nil)
		r1.On("DeleteOnSuspend").Return(false)
		r1.On("Suspend").Return(errors.New("fail1"))
		r2 := &MockSuspendableResource{}
		r2.On("Object").Return(obj2, nil)
		r2.On("DeleteOnSuspend").Return(false)
		r2.On("Suspend").Return(errors.New("fail2"))

		resources := []reconcileEntry{{Resource: r1}, {Resource: r2}}
		results, err := suspendResources(ctx, rec, resources, "test-component", testRESTMapper())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "fail1")
		assert.Contains(t, err.Error(), "fail2")
		assert.Nil(t, results)
	})

	t.Run("should return empty results if no suspendable resources", func(t *testing.T) {
		rec := setupReconcileContext(scheme, nil, &MockClient{})
		nonSuspendable := &MockResource{}

		resources := []reconcileEntry{{Resource: nonSuspendable}}
		results, err := suspendResources(ctx, rec, resources, "test-component", testRESTMapper())

		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func TestSuspendResource_Unowned(t *testing.T) {
	ctx := t.Context()
	scheme := setupScheme()

	t.Run("does not set owner reference on Unowned resource during suspension", func(t *testing.T) {
		owner := setupTestOwner()
		res, obj := setupMockResource("cm-unowned-suspend", concepts.SuspensionStatusSuspended, "Done", false)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
		rec := setupReconcileContext(scheme, owner, cli)

		entry := reconcileEntry{
			Resource: res,
			Options:  resourceOptions{Unowned: true, ParticipationMode: ParticipationModeRequired},
		}

		status, err := suspendResource(ctx, rec, entry, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)

		created := &v1.ConfigMap{}
		err = cli.Get(ctx, client.ObjectKeyFromObject(obj), created)
		require.NoError(t, err)
		assert.Empty(t, created.OwnerReferences, "Unowned resource must not have an owner reference during suspension")
	})
}

func TestSuspendResource(t *testing.T) {
	ctx := t.Context()
	scheme := setupScheme()

	t.Run("successful suspension without deletion", func(t *testing.T) {
		owner := setupTestOwner()
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", false)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj).Build()
		rec := setupReconcileContext(scheme, owner, cli)

		status, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)

		// Verify object still exists
		err = cli.Get(ctx, client.ObjectKeyFromObject(obj), &v1.ConfigMap{})
		require.NoError(t, err)
	})

	t.Run("successful suspension with deletion", func(t *testing.T) {
		owner := setupTestOwner()
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", true)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj).Build()
		rec := setupReconcileContext(scheme, owner, cli)

		status, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)

		// Verify object is deleted
		err = cli.Get(ctx, client.ObjectKeyFromObject(obj), &v1.ConfigMap{})
		assert.True(t, apierrors.IsNotFound(err))

		recorder, ok := rec.EventRecorder.(*spyRecorder)
		require.True(t, ok)

		deletions := recorder.recordedWithReason("ResourceDeleted")
		require.Len(t, deletions, 1)
		assert.Equal(t, owner, deletions[0].regarding, "event is recorded on the owner")
		assert.Equal(t, obj.GetName(), deletions[0].related.(client.Object).GetName(),
			"deleted resource is attached as the related object")
		assert.Equal(t, v1.EventTypeNormal, deletions[0].eventType)
		assert.Equal(t, "Delete", deletions[0].action)
		assert.Contains(t, deletions[0].note, "deleted on suspension")
	})

	t.Run("should not delete if suspension is not completed", func(t *testing.T) {
		owner := setupTestOwner()
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspending, "Wait", true)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj).Build()
		rec := setupReconcileContext(scheme, owner, cli)

		status, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspending, status.Status)

		// Verify object still exists
		err = cli.Get(ctx, client.ObjectKeyFromObject(obj), &v1.ConfigMap{})
		require.NoError(t, err)
	})

	t.Run("should handle not found on deletion", func(t *testing.T) {
		owner := setupTestOwner()
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", true)

		cli := &MockClient{}
		rec := setupReconcileContext(scheme, owner, cli)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Patch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Delete", ctx, obj, mock.Anything).Return(apierrors.NewNotFound(schema.GroupResource{}, "cm"))

		status, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})

	t.Run("should return error if Suspend fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, nil, cli)
		res := &MockSuspendableResource{}
		obj := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"}}
		res.On("Object").Return(obj, nil)
		res.On("DeleteOnSuspend").Return(false)
		res.On("Suspend").Return(errors.New("suspend fail"))

		_, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "suspend fail")
	})

	t.Run("should return error if resource.Object() fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, nil, cli)
		res := &MockSuspendableResource{}
		res.On("Object").Return(nil, errors.New("object fail"))

		_, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "object fail")
	})

	t.Run("should return error if applyResources fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, _ := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", false)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("get fail"))

		_, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get fail")
	})

	t.Run("should return error if SuspensionStatus fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, _ := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", false)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Patch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		res.On("SuspensionStatus").Unset()
		res.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{}, errors.New("status fail"))

		_, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status fail")
	})

	t.Run("should return error if Delete fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", true)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Patch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Delete", ctx, obj, mock.Anything).Return(errors.New("delete fail"))

		_, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete fail")
	})

	t.Run("should not return error if Delete returns NotFound", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", true)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Patch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Delete", ctx, obj, mock.Anything).Return(apierrors.NewNotFound(schema.GroupResource{Group: "v1", Resource: "ConfigMap"}, "cm"))

		status, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
		cli.AssertCalled(t, "Delete", ctx, obj, mock.Anything)
	})

	t.Run("should skip Apply when DeleteOnSuspend is true and object does not exist", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, _ := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", true)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, "cm"))

		status, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", nil)
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
		assert.Contains(t, status.Reason, "already deleted")

		// Verify Suspend() was not called — no mutation should be queued when short-circuiting
		res.AssertNotCalled(t, "Suspend")
		// Verify Apply was never called (no Patch or Create calls)
		cli.AssertNotCalled(t, "Patch", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		cli.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything)
		cli.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("should return error if existence check fails for DeleteOnSuspend resource", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, _ := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", true)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(errors.New("network error"))

		_, err := suspendResource(ctx, rec, reconcileEntry{Resource: res}, res, "test-component", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network error")
	})
}

func TestSuspendResource_BlockOnForeignController(t *testing.T) {
	ctx := t.Context()
	scheme := setupScheme()
	foreign := metav1.OwnerReference{
		APIVersion: "test/v1", Kind: "MockOperatorCRD", Name: "other-owner", UID: "other-uid",
		Controller: ptr.To(true),
	}

	t.Run("reports Suspended without touching an object another owner controls", func(t *testing.T) {
		owner := setupTestOwner()
		owner.UID = "this-uid"
		res := &MockSuspendableResource{}
		desired := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"}}
		res.On("Object").Return(desired, nil)
		res.On("Identity").Return("ConfigMap/cm")
		res.On("DeleteOnSuspend").Return(true)
		// Suspend, Mutate and SuspensionStatus carry no expectations: reaching any of
		// them means the foreign object was about to be applied or deleted.

		live := desired.DeepCopy()
		live.OwnerReferences = []metav1.OwnerReference{foreign}
		live.Data = map[string]string{"owner": "other-owner"}
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, live).Build()
		rec := setupReconcileContext(scheme, owner, cli)
		entry := reconcileEntry{Resource: res, Options: resourceOptions{BlockOnForeignController: true}}

		status, err := suspendResource(ctx, rec, entry, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
		assert.Contains(t, status.Reason, "controlled by MockOperatorCRD other-owner")

		got := &v1.ConfigMap{}
		require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(desired), got))
		assert.Equal(t, live.Data, got.Data)
		assert.Equal(t, live.OwnerReferences, got.OwnerReferences)
	})

	t.Run("suspends an object this owner controls", func(t *testing.T) {
		owner := setupTestOwner()
		owner.UID = "this-uid"
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", false)
		obj.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: GroupVersion.String(), Kind: "MockOperatorCRD", Name: owner.Name, UID: owner.UID, Controller: ptr.To(true),
		}}
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj).Build()
		rec := setupReconcileContext(scheme, owner, cli)
		entry := reconcileEntry{Resource: res, Options: resourceOptions{BlockOnForeignController: true}}

		status, err := suspendResource(ctx, rec, entry, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
		assert.Equal(t, "Done", status.Reason)
		res.AssertCalled(t, "Suspend")
	})

	t.Run("suspends an object with no controller", func(t *testing.T) {
		owner := setupTestOwner()
		owner.UID = "this-uid"
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", false)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj).Build()
		rec := setupReconcileContext(scheme, owner, cli)
		entry := reconcileEntry{Resource: res, Options: resourceOptions{BlockOnForeignController: true}}

		status, err := suspendResource(ctx, rec, entry, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
		res.AssertCalled(t, "Suspend")
	})
}

func TestSuspendResource_BlockOnForeignController_DeleteOnSuspend(t *testing.T) {
	ctx := t.Context()
	scheme := setupScheme()

	t.Run("does not delete on suspend an object another owner claims after it was observed as safe", func(t *testing.T) {
		owner := setupTestOwner()
		owner.UID = "this-uid"
		res, obj := setupMockResource("claimed", concepts.SuspensionStatusSuspended, "Done", true)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).
			WithInterceptorFuncs(claimBeforeDelete(t, "claimed")).Build()
		rec := setupReconcileContext(scheme, owner, cli)
		entry := reconcileEntry{Resource: res, Options: resourceOptions{Unowned: true, BlockOnForeignController: true}}

		_, err := suspendResource(ctx, rec, entry, res, "test-component", testRESTMapper())
		require.Error(t, err)
		got := &v1.ConfigMap{}
		require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(obj), got))
		assert.Equal(t, "other-uid", string(got.OwnerReferences[0].UID))
	})

	t.Run("deletes on suspend an object it observed as safe", func(t *testing.T) {
		owner := setupTestOwner()
		owner.UID = "this-uid"
		res, obj := setupMockResource("safe", concepts.SuspensionStatusSuspended, "Done", true)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj.DeepCopy()).Build()
		rec := setupReconcileContext(scheme, owner, cli)
		entry := reconcileEntry{Resource: res, Options: resourceOptions{Unowned: true, BlockOnForeignController: true}}

		status, err := suspendResource(ctx, rec, entry, res, "test-component", testRESTMapper())
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
		assert.True(t, apierrors.IsNotFound(cli.Get(ctx, client.ObjectKeyFromObject(obj), &v1.ConfigMap{})))
	})
}
