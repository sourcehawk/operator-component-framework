package component

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
)

type MockMetrics struct {
	mock.Mock
}

func (m *MockMetrics) RecordConditionFor(
	kind string, object ocm.ObjectLike,
	conditionType, conditionStatus, conditionReason string, lastTransitionTime time.Time,
	extraLabelValues ...string,
) {
	m.Called(kind, object, conditionType, conditionStatus, conditionReason, lastTransitionTime, extraLabelValues)
}

func TestConvergingCondition(t *testing.T) {
	componentType := ConditionType("TestComponent")
	observedGen := int64(1)

	tests := []struct {
		name     string
		status   convergingStatus
		expected metav1.ConditionStatus
		reason   string
	}{
		{"AliveHealthy", convergingStatusAliveHealthy, metav1.ConditionTrue, "Healthy"},
		{"OperationalOperational", convergingStatusOperationalOperational, metav1.ConditionTrue, "Operational"},
		{"CompletableCompleted", convergingStatusCompletableCompleted, metav1.ConditionTrue, "Completed"},
		{"AliveCreating", convergingStatusAliveCreating, metav1.ConditionFalse, "Creating"},
		{"AliveUpdating", convergingStatusAliveUpdating, metav1.ConditionFalse, "Updating"},
		{"AliveScaling", convergingStatusAliveScaling, metav1.ConditionFalse, "Scaling"},
		{"AliveFailing", convergingStatusAliveFailing, metav1.ConditionFalse, "Failing"},
		{"OperationalPending", convergingStatusOperationalPending, metav1.ConditionFalse, "OperationPending"},
		{"OperationalFailing", convergingStatusOperationalFailing, metav1.ConditionFalse, "OperationFailing"},
		{"CompletablePending", convergingStatusCompletablePending, metav1.ConditionFalse, "TaskPending"},
		{"CompletableRunning", convergingStatusCompletableRunning, metav1.ConditionFalse, "TaskRunning"},
		{"CompletableFailed", convergingStatusCompletableFailed, metav1.ConditionFalse, "TaskFailing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converging := convergingStatusWithReason{
				Status: tt.status,
				Reason: "status explanation",
			}
			cond := convergingCondition(componentType, converging, observedGen)

			assert.Equal(t, string(componentType), cond.Type)
			assert.Equal(t, tt.expected, cond.Status)
			assert.Equal(t, tt.reason, cond.Reason)
			assert.Equal(t, converging.Reason, cond.Message)
			assert.Equal(t, observedGen, cond.ObservedGeneration)
		})
	}
}

func TestSuspendingCondition(t *testing.T) {
	componentType := ConditionType("TestComponent")
	observedGen := int64(1)

	t.Run("Suspended status", func(t *testing.T) {
		suspending := concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspended,
			Reason: "Component suspended",
		}
		cond := suspendingCondition(componentType, suspending, observedGen)

		assert.Equal(t, string(componentType), cond.Type)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Suspended), cond.Reason)
		assert.Equal(t, suspending.Reason, cond.Message)
		assert.Equal(t, observedGen, cond.ObservedGeneration)
	})

	t.Run("Suspending status", func(t *testing.T) {
		suspending := concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspending,
			Reason: "Component is suspending",
		}
		cond := suspendingCondition(componentType, suspending, observedGen)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(Suspending), cond.Reason)
	})
}

func TestGraceCondition(t *testing.T) {
	componentType := ConditionType("TestComponent")
	observedGen := int64(1)

	t.Run("Degraded status", func(t *testing.T) {
		gracing := concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Something is wrong",
		}
		cond := graceCondition(componentType, gracing, observedGen)

		assert.Equal(t, string(componentType), cond.Type)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(Degraded), cond.Reason)
		assert.Contains(t, cond.Message, "Component is degraded: ")
		assert.Contains(t, cond.Message, gracing.Reason)
		assert.Equal(t, observedGen, cond.ObservedGeneration)
	})

	t.Run("Down status", func(t *testing.T) {
		gracing := concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDown,
			Reason: "Everything is wrong",
		}
		cond := graceCondition(componentType, gracing, observedGen)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(Down), cond.Reason)
		assert.Contains(t, cond.Message, "Component is down: ")
	})
}

func TestStaticConditions(t *testing.T) {
	componentType := ConditionType("TestComponent")
	observedGen := int64(1)

	t.Run("conditionReady", func(t *testing.T) {
		cond := conditionReady(componentType, observedGen)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Healthy), cond.Reason)
	})

	t.Run("conditionError", func(t *testing.T) {
		err := errors.New("reconcile failed")
		cond := conditionError(componentType, err, observedGen)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(Error), cond.Reason)
		assert.Equal(t, err.Error(), cond.Message)
	})

	t.Run("conditionUnknown", func(t *testing.T) {
		cond := conditionUnknown(componentType, observedGen)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(Unknown), cond.Reason)
	})

	t.Run("conditionDisabled", func(t *testing.T) {
		cond := conditionDisabled(componentType, observedGen)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Disabled), cond.Reason)
		assert.Equal(t, "Component is disabled.", cond.Message)
		assert.Equal(t, observedGen, cond.ObservedGeneration)
	})

	t.Run("conditionPrerequisiteNotMet", func(t *testing.T) {
		reason := "Condition DepReady is not True"
		cond := conditionPrerequisiteNotMet(componentType, reason, observedGen)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(PrerequisiteNotMet), cond.Reason)
		assert.Equal(t, reason, cond.Message)
		assert.Equal(t, observedGen, cond.ObservedGeneration)
	})
}

func TestConditionMethods(t *testing.T) {
	cond := Condition{
		Type:   "TestComponent",
		Reason: string(Healthy),
		Status: metav1.ConditionTrue,
	}

	t.Run("ConditionType", func(t *testing.T) {
		assert.Equal(t, ConditionType("TestComponent"), cond.ConditionType())
	})

	t.Run("ComponentStatus", func(t *testing.T) {
		assert.Equal(t, Healthy, cond.ComponentStatus())
	})
}

func TestApplyStatusCondition(t *testing.T) {
	owner := &MockOperatorCRD{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-owner",
			Namespace:  "default",
			Generation: 1,
		},
	}

	rec := ReconcileContext{
		Client:  failingClient{},
		Metrics: metricsThatPanic{},
		Owner:   owner,
	}

	cond := Condition{
		Type:               "TestComponent",
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		Message:            "Ready to go",
		ObservedGeneration: 1,
	}

	// Pure in-memory mutation: no client call, no metrics call. Using a client
	// that would fail on any API access and a metrics recorder that would
	// panic on any invocation proves applyStatusCondition never reaches either.
	applyStatusCondition(rec, cond)

	conditions := owner.GetStatusConditions()
	require.Len(t, *conditions, 1)
	assert.Equal(t, cond.Type, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
	assert.Equal(t, cond.Reason, (*conditions)[0].Reason)
}

func TestFlushStatus(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))

	newOwner := func() *MockOperatorCRD {
		return &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-owner",
				Namespace:  "default",
				Generation: 1,
			},
		}
	}

	cond := func(ctype, reason string, status metav1.ConditionStatus) Condition {
		return Condition{Type: ctype, Status: status, Reason: reason, ObservedGeneration: 1}
	}

	t.Run("persists every condition on the owner and records a metric per condition", func(t *testing.T) {
		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, cond("InfraReady", "Ready", metav1.ConditionTrue))
		applyStatusCondition(ReconcileContext{Owner: owner}, cond("AppReady", "Ready", metav1.ConditionTrue))

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()
		metrics := &MockMetrics{}
		metrics.On("RecordConditionFor", owner.GetKind(), owner, "InfraReady",
			string(metav1.ConditionTrue), "Ready", mock.Anything, mock.Anything).Return().Once()
		metrics.On("RecordConditionFor", owner.GetKind(), owner, "AppReady",
			string(metav1.ConditionTrue), "Ready", mock.Anything, mock.Anything).Return().Once()

		require.NoError(t, FlushStatus(ctx, ReconcileContext{Client: k8sClient, Metrics: metrics, Owner: owner}))

		persisted := &MockOperatorCRD{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), persisted))
		assert.Len(t, persisted.Status.Conditions, 2)
		metrics.AssertExpectations(t)
	})

	t.Run("is a no-op when metrics recorder is nil", func(t *testing.T) {
		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, cond("InfraReady", "Ready", metav1.ConditionTrue))

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()

		require.NoError(t, FlushStatus(ctx, ReconcileContext{Client: k8sClient, Owner: owner}))

		persisted := &MockOperatorCRD{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), persisted))
		assert.Len(t, persisted.Status.Conditions, 1)
	})

	t.Run("surfaces non-conflict update errors without recording metrics", func(t *testing.T) {
		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, cond("InfraReady", "Ready", metav1.ConditionTrue))

		inner := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()
		k8sClient := &errorMockClient{Client: inner}
		metrics := &MockMetrics{}

		err := FlushStatus(ctx, ReconcileContext{Client: k8sClient, Metrics: metrics, Owner: owner})
		require.Error(t, err)
		assert.Equal(t, "update failed", err.Error())

		metrics.AssertNotCalled(t, "RecordConditionFor")
	})

	t.Run("retries on conflict by refetching the owner and reapplying staged conditions", func(t *testing.T) {
		owner := newOwner()

		// External writer got there first with a condition the framework does not manage.
		serverSide := newOwner()
		serverSide.Status.Conditions = []metav1.Condition{{
			Type:               "ExternalReady",
			Status:             metav1.ConditionTrue,
			Reason:             "ExternalReason",
			LastTransitionTime: metav1.Now(),
		}}
		inner := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(owner).WithObjects(serverSide).Build()
		// The in-memory owner has a stale ResourceVersion (empty), so the first
		// Update must conflict. The conflict wrapper refetches, reapplies our
		// staged condition, and lets retry.RetryOnConflict succeed on the retry.
		k8sClient := &conflictOnceClient{Client: inner}

		applyStatusCondition(ReconcileContext{Owner: owner}, cond("InfraReady", "Ready", metav1.ConditionTrue))

		metrics := &MockMetrics{}
		metrics.On("RecordConditionFor", owner.GetKind(), owner, "ExternalReady",
			string(metav1.ConditionTrue), "ExternalReason", mock.Anything, mock.Anything).Return().Once()
		metrics.On("RecordConditionFor", owner.GetKind(), owner, "InfraReady",
			string(metav1.ConditionTrue), "Ready", mock.Anything, mock.Anything).Return().Once()

		require.NoError(t, FlushStatus(ctx, ReconcileContext{Client: k8sClient, Metrics: metrics, Owner: owner}))

		assert.GreaterOrEqual(t, k8sClient.gets, 1, "expected at least one Get for conflict refetch")
		persisted := &MockOperatorCRD{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), persisted))
		// Both the externally written condition and the framework-managed
		// condition must be present on the persisted owner.
		types := make([]string, 0, len(persisted.Status.Conditions))
		for _, c := range persisted.Status.Conditions {
			types = append(types, c.Type)
		}
		assert.ElementsMatch(t, []string{"ExternalReady", "InfraReady"}, types)
		metrics.AssertExpectations(t)
	})
}

type errorMockClient struct {
	client.Client
}

func (e *errorMockClient) Status() client.SubResourceWriter {
	return &errorStatusWriter{SubResourceWriter: e.Client.Status()}
}

type errorStatusWriter struct {
	client.SubResourceWriter
}

func (e *errorStatusWriter) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return errors.New("update failed")
}

// conflictOnceClient wraps a real client and causes the first status Update to
// return a Conflict error, forcing FlushStatus to refetch and retry. Subsequent
// Updates fall through to the underlying client.
type conflictOnceClient struct {
	client.Client
	conflicts int
	gets      int
}

func (c *conflictOnceClient) Status() client.SubResourceWriter {
	return &conflictOnceStatusWriter{SubResourceWriter: c.Client.Status(), parent: c}
}

func (c *conflictOnceClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	c.gets++
	return c.Client.Get(ctx, key, obj, opts...)
}

type conflictOnceStatusWriter struct {
	client.SubResourceWriter
	parent *conflictOnceClient
}

func (w *conflictOnceStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if w.parent.conflicts == 0 {
		w.parent.conflicts++
		return apierrors.NewConflict(
			schema.GroupResource{Group: "example.io", Resource: "mockoperatorcrds"},
			obj.GetName(),
			fmt.Errorf("the object has been modified; please apply your changes to the latest version and try again"),
		)
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

// failingClient is a client whose every method returns an error. Used to prove
// that applyStatusCondition never calls the API.
type failingClient struct {
	client.Client
}

func (failingClient) Status() client.SubResourceWriter {
	panic("applyStatusCondition must not reach the client")
}
func (failingClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	panic("applyStatusCondition must not reach the client")
}

// metricsThatPanic is a Recorder whose every method panics. Used to prove that
// applyStatusCondition never records metrics.
type metricsThatPanic struct{}

func (metricsThatPanic) RecordConditionFor(string, ocm.ObjectLike, string, string, string, time.Time, ...string) {
	panic("applyStatusCondition must not record metrics")
}
