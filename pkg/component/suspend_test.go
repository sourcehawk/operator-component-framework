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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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

		resources := []Resource{r1, r2, nonSuspendable}
		results, err := suspendResources(ctx, rec, resources)

		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, concepts.SuspensionStatusSuspended, results[0].Status)
		assert.Equal(t, concepts.SuspensionStatusSuspending, results[1].Status)
	})

	t.Run("should return joined errors if any suspension fails", func(t *testing.T) {
		rec := setupReconcileContext(scheme, nil, &MockClient{})
		r1 := &MockSuspendableResource{}
		r1.On("Suspend").Return(errors.New("fail1"))
		r2 := &MockSuspendableResource{}
		r2.On("Suspend").Return(errors.New("fail2"))

		resources := []Resource{r1, r2}
		results, err := suspendResources(ctx, rec, resources)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "fail1")
		assert.Contains(t, err.Error(), "fail2")
		assert.Nil(t, results)
	})

	t.Run("should return empty results if no suspendable resources", func(t *testing.T) {
		rec := setupReconcileContext(scheme, nil, &MockClient{})
		nonSuspendable := &MockResource{}

		resources := []Resource{nonSuspendable}
		results, err := suspendResources(ctx, rec, resources)

		require.NoError(t, err)
		assert.Empty(t, results)
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

		status, err := suspendResource(ctx, rec, res, res)
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

		status, err := suspendResource(ctx, rec, res, res)
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)

		// Verify object is deleted
		err = cli.Get(ctx, client.ObjectKeyFromObject(obj), &v1.ConfigMap{})
		assert.True(t, apierrors.IsNotFound(err))

		recorder := rec.Recorder.(*record.FakeRecorder)

		// Drain the "UpdatedConfigMap" event from createOrUpdateResources
		select {
		case <-recorder.Events:
		default:
		}

		select {
		case event := <-recorder.Events:
			assert.Contains(t, event, "ResourceDeleted")
		default:
			t.Fatal("expected event but none found")
		}
	})

	t.Run("should not delete if suspension is not completed", func(t *testing.T) {
		owner := setupTestOwner()
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspending, "Wait", true)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, obj).Build()
		rec := setupReconcileContext(scheme, owner, cli)

		status, err := suspendResource(ctx, rec, res, res)
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
		cli.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Delete", ctx, obj, mock.Anything).Return(apierrors.NewNotFound(schema.GroupResource{}, "cm"))

		status, err := suspendResource(ctx, rec, res, res)
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})

	t.Run("should return error if Suspend fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, nil, cli)
		res := &MockSuspendableResource{}
		res.On("Suspend").Return(errors.New("suspend fail"))

		_, err := suspendResource(ctx, rec, res, res)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "suspend fail")
	})

	t.Run("should return error if resource.Object() fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, nil, cli)
		res := &MockSuspendableResource{}
		res.On("Suspend").Return(nil)
		res.On("Object").Return(nil, errors.New("object fail"))

		_, err := suspendResource(ctx, rec, res, res)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "object fail")
	})

	t.Run("should return error if createOrUpdateResources fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, _ := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", false)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("get fail"))

		_, err := suspendResource(ctx, rec, res, res)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get fail")
	})

	t.Run("should return error if SuspensionStatus fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, _ := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", false)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		res.On("SuspensionStatus").Unset()
		res.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{}, errors.New("status fail"))

		_, err := suspendResource(ctx, rec, res, res)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status fail")
	})

	t.Run("should return error if Delete fails", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", true)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Delete", ctx, obj, mock.Anything).Return(errors.New("delete fail"))

		_, err := suspendResource(ctx, rec, res, res)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete fail")
	})

	t.Run("should not return error if Delete returns NotFound", func(t *testing.T) {
		cli := &MockClient{}
		rec := setupReconcileContext(scheme, setupTestOwner(), cli)
		res, obj := setupMockResource("cm", concepts.SuspensionStatusSuspended, "Done", true)

		cli.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		cli.On("Delete", ctx, obj, mock.Anything).Return(apierrors.NewNotFound(schema.GroupResource{Group: "v1", Resource: "ConfigMap"}, "cm"))

		status, err := suspendResource(ctx, rec, res, res)
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
		cli.AssertCalled(t, "Delete", ctx, obj, mock.Anything)
	})
}
