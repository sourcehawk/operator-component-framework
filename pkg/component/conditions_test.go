package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

func (m *MockMetrics) RecordResourceApply(labels ResourceMetricLabels, operation concepts.ConvergingOperation) {
	m.Called(labels, operation)
}

func (m *MockMetrics) RecordResourceApplyError(labels ResourceMetricLabels) {
	m.Called(labels)
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

		require.NoError(t, FlushStatus(ctx, ReconcileContext{Client: k8sClient, Metrics: metrics, Owner: owner}, nil))

		persisted := &MockOperatorCRD{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), persisted))
		assert.Len(t, persisted.Status.Conditions, 2)
		metrics.AssertExpectations(t)
	})

	t.Run("is a no-op when metrics recorder is nil", func(t *testing.T) {
		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, cond("InfraReady", "Ready", metav1.ConditionTrue))

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()

		require.NoError(t, FlushStatus(ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil))

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

		err := FlushStatus(ctx, ReconcileContext{Client: k8sClient, Metrics: metrics, Owner: owner}, nil)
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

		infra, err := NewComponentBuilder().WithName("infra").WithConditionType("InfraReady").Build()
		require.NoError(t, err)

		metrics := &MockMetrics{}
		metrics.On("RecordConditionFor", owner.GetKind(), owner, "ExternalReady",
			string(metav1.ConditionTrue), "ExternalReason", mock.Anything, mock.Anything).Return().Once()
		metrics.On("RecordConditionFor", owner.GetKind(), owner, "InfraReady",
			string(metav1.ConditionTrue), "Ready", mock.Anything, mock.Anything).Return().Once()

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Metrics: metrics, Owner: owner}, []*Component{infra},
		))

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

// failingClient is a client whose every method panics. Used to prove that
// applyStatusCondition never calls the API.
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

func (metricsThatPanic) RecordResourceApply(ResourceMetricLabels, concepts.ConvergingOperation) {
	panic("applyStatusCondition must not record metrics")
}

func (metricsThatPanic) RecordResourceApplyError(ResourceMetricLabels) {
	panic("applyStatusCondition must not record metrics")
}

// TestFlushStatusConflictOwnership covers the conflict path, where FlushStatus
// must keep the staged owner as the object being written and take the server's
// value only for condition types the framework does not own.
func TestFlushStatusConflictOwnership(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))

	newOwner := func() *MockOperatorCRD {
		return &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: "default", Generation: 1},
		}
	}

	// infraComponent owns the "InfraReady" condition type.
	infraComponent := func(t *testing.T) *Component {
		t.Helper()
		comp, err := NewComponentBuilder().WithName("infra").WithConditionType("InfraReady").Build()
		require.NoError(t, err)
		return comp
	}

	// conflictingClient returns a client whose first status Update conflicts,
	// backed by a server object in the given state.
	conflictingClient := func(serverSide *MockOperatorCRD) *conflictOnceClient {
		inner := fake.NewClientBuilder().
			WithScheme(scheme).WithStatusSubresource(serverSide).WithObjects(serverSide).Build()
		return &conflictOnceClient{Client: inner}
	}

	persistedOf := func(t *testing.T, c client.Client, owner *MockOperatorCRD) *MockOperatorCRD {
		t.Helper()
		persisted := &MockOperatorCRD{}
		require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(owner), persisted))
		return persisted
	}

	conditionOf := func(t *testing.T, o *MockOperatorCRD, ctype string) metav1.Condition {
		t.Helper()
		found := meta.FindStatusCondition(o.Status.Conditions, ctype)
		require.NotNil(t, found, "expected condition %q on the persisted owner", ctype)
		return *found
	}

	t.Run("preserves a staged non-condition status field across a conflict", func(t *testing.T) {
		serverSide := newOwner()
		serverSide.Status.ObservedGeneration = 0
		k8sClient := conflictingClient(serverSide)

		owner := newOwner()
		owner.Status.ObservedGeneration = 7 // staged by the controller, not a condition
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
		))

		assert.Equal(t, int64(7), persistedOf(t, k8sClient, owner).Status.ObservedGeneration,
			"the staged owner must remain the object written, so its non-condition fields survive")
	})

	t.Run("keeps our staged value for a condition type we own", func(t *testing.T) {
		serverSide := newOwner()
		serverSide.Status.Conditions = []metav1.Condition{{
			Type: "InfraReady", Status: metav1.ConditionFalse, Reason: "StaleServerValue",
			LastTransitionTime: metav1.Now(),
		}}
		k8sClient := conflictingClient(serverSide)

		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
		))

		got := conditionOf(t, persistedOf(t, k8sClient, owner), "InfraReady")
		assert.Equal(t, "Healthy", got.Reason, "an owned type must keep the value we staged")
		assert.Equal(t, metav1.ConditionTrue, got.Status)
	})

	t.Run("keeps an owned condition removed from the staged owner removed across a conflict", func(t *testing.T) {
		// This is what makes component retirement conflict-safe: the retired
		// component is still passed for the pass that removes its condition, so
		// the type is owned and the conflict refresh must not restore it from the
		// server. Only unowned types follow the server.
		serverSide := newOwner()
		serverSide.Status.Conditions = []metav1.Condition{{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
			LastTransitionTime: metav1.Now(),
		}}
		k8sClient := conflictingClient(serverSide)

		owner := newOwner() // InfraReady deliberately absent: the controller removed it

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Nil(t, meta.FindStatusCondition(persisted.Status.Conditions, "InfraReady"),
			"an owned condition removed from the staged owner must stay removed; the server copy must not be restored")
	})

	t.Run("reads the server through APIReader on conflict, not through the cached client", func(t *testing.T) {
		// A controller's Client reads from the informer cache, which can lag the
		// write that caused the 409 and hand back the same stale resourceVersion.
		// The conflict path must read through APIReader when one is set. Model
		// the lag by giving the client and the reader different views of the same
		// object: only the reader knows the other writer's newer condition.
		serverSide := newOwner()
		serverSide.Status.Conditions = []metav1.Condition{{
			Type: "ExternalReady", Status: metav1.ConditionTrue, Reason: "SeenOnlyByAPIReader",
			LastTransitionTime: metav1.Now(),
		}}
		apiReader := fake.NewClientBuilder().
			WithScheme(scheme).WithStatusSubresource(serverSide).WithObjects(serverSide).Build()

		cachedView := newOwner() // the cache never saw ExternalReady at all
		k8sClient := conflictingClient(cachedView)

		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx,
			ReconcileContext{Client: k8sClient, APIReader: apiReader, Owner: owner},
			[]*Component{infraComponent(t)},
		))

		assert.Equal(t, 0, k8sClient.gets, "the conflict refetch must not go through the cached client")
		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, "SeenOnlyByAPIReader", conditionOf(t, persisted, "ExternalReady").Reason,
			"the unowned condition must come from the direct read")
		assert.Equal(t, "Healthy", conditionOf(t, persisted, "InfraReady").Reason)
	})

	t.Run("takes the server's newer value for a condition type we do not own", func(t *testing.T) {
		serverSide := newOwner()
		serverSide.Status.Conditions = []metav1.Condition{{
			Type: "ExternalReady", Status: metav1.ConditionTrue, Reason: "WrittenByOtherController",
			LastTransitionTime: metav1.Now(),
		}}
		k8sClient := conflictingClient(serverSide)

		// Our in-memory owner carries a stale copy of the other writer's condition,
		// exactly as it would after fetching the owner at the top of the reconcile.
		owner := newOwner()
		owner.Status.Conditions = []metav1.Condition{{
			Type: "ExternalReady", Status: metav1.ConditionTrue, Reason: "StaleValueFromOurGet",
			LastTransitionTime: metav1.Now(),
		}}
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, "WrittenByOtherController", conditionOf(t, persisted, "ExternalReady").Reason,
			"a type we do not own must take the server's value, not our stale copy")
		assert.Equal(t, "Healthy", conditionOf(t, persisted, "InfraReady").Reason)
	})

	t.Run("takes the server's LastTransitionTime for an unowned condition, even when status is unchanged", func(t *testing.T) {
		// This is the case that tells replaceStatusCondition apart from
		// meta.SetStatusCondition. SetStatusCondition keeps the existing
		// LastTransitionTime whenever the status did not change, so it would blend
		// the server's condition with the stale local timestamp. The refresh must
		// take the server's condition verbatim, transition time included.
		serverTransition := metav1.NewTime(time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC))
		staleTransition := metav1.NewTime(serverTransition.Add(-time.Hour))

		serverSide := newOwner()
		serverSide.Status.Conditions = []metav1.Condition{{
			Type: "ExternalReady", Status: metav1.ConditionTrue, Reason: "Same",
			LastTransitionTime: serverTransition,
		}}
		k8sClient := conflictingClient(serverSide)

		owner := newOwner()
		owner.Status.Conditions = []metav1.Condition{{
			Type: "ExternalReady", Status: metav1.ConditionTrue, Reason: "Same", // same status and reason
			LastTransitionTime: staleTransition,
		}}
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
		))

		got := conditionOf(t, persistedOf(t, k8sClient, owner), "ExternalReady")
		assert.True(t, got.LastTransitionTime.Equal(&serverTransition),
			"an unowned condition must carry the server's LastTransitionTime, not the stale local one; got %v want %v",
			got.LastTransitionTime, serverTransition)
	})

	t.Run("preserves a condition another writer added after our fetch", func(t *testing.T) {
		serverSide := newOwner()
		serverSide.Status.Conditions = []metav1.Condition{{
			Type: "AddedAfterOurGet", Status: metav1.ConditionTrue, Reason: "BrandNew",
			LastTransitionTime: metav1.Now(),
		}}
		k8sClient := conflictingClient(serverSide)

		owner := newOwner() // never saw AddedAfterOurGet
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, "BrandNew", conditionOf(t, persisted, "AddedAfterOurGet").Reason)
		assert.Equal(t, "Healthy", conditionOf(t, persisted, "InfraReady").Reason)
	})

	t.Run("owns nothing when no components are passed", func(t *testing.T) {
		// You own exactly what you pass. A validation-only controller passes nil, so
		// its hand-staged condition is unowned and follows the server on a conflict,
		// exactly as a controller-staged aggregate does. The next reconcile stages it
		// again. Its non-condition status fields still survive, because the staged
		// owner is never replaced.
		serverSide := newOwner()
		serverSide.Status.Conditions = []metav1.Condition{{
			Type: "Validated", Status: metav1.ConditionFalse, Reason: "StaleServerValue",
			LastTransitionTime: metav1.Now(),
		}}
		k8sClient := conflictingClient(serverSide)

		owner := newOwner()
		owner.Status.ObservedGeneration = 3
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "Validated", Status: metav1.ConditionTrue, Reason: "SpecValid",
		})

		require.NoError(t, FlushStatus(ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, "StaleServerValue", conditionOf(t, persisted, "Validated").Reason,
			"nothing is owned, so the server's value wins on a conflict")
		assert.Equal(t, int64(3), persisted.Status.ObservedGeneration,
			"a staged non-condition status field still survives")
	})

	t.Run("treats a nil and an empty component slice identically", func(t *testing.T) {
		// A reader will reasonably write either. Both own no condition types at
		// all, so both must follow the server the same way on a conflict.
		for name, comps := range map[string][]*Component{
			"nil":           nil,
			"empty non-nil": {},
		} {
			t.Run(name, func(t *testing.T) {
				serverSide := newOwner()
				serverSide.Status.Conditions = []metav1.Condition{{
					Type: "Validated", Status: metav1.ConditionFalse, Reason: "StaleServerValue",
					LastTransitionTime: metav1.Now(),
				}}
				k8sClient := conflictingClient(serverSide)

				owner := newOwner()
				owner.Status.ObservedGeneration = 5
				applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
					Type: "Validated", Status: metav1.ConditionTrue, Reason: "SpecValid",
				})

				require.NoError(t, FlushStatus(
					ctx, ReconcileContext{Client: k8sClient, Owner: owner}, comps,
				))

				persisted := persistedOf(t, k8sClient, owner)
				assert.Equal(t, "StaleServerValue", conditionOf(t, persisted, "Validated").Reason,
					"nothing is owned either way")
				assert.Equal(t, int64(5), persisted.Status.ObservedGeneration)
			})
		}
	})

	t.Run("drops an unowned condition the server no longer carries", func(t *testing.T) {
		// Absence is server state. Another writer removing its condition must not
		// be undone by this controller writing its stale local copy back.
		//
		// This also pins the cleared server copy: the fetch decodes into a separate
		// object whose conditions are emptied first, so a response that omits
		// conditions cannot leave the staged ones behind looking like the server's.
		serverSide := newOwner() // the other writer deleted its condition
		k8sClient := conflictingClient(serverSide)

		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "ExternalReady", Status: metav1.ConditionTrue, Reason: "StaleLocalCopy",
		})
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, "Healthy", conditionOf(t, persisted, "InfraReady").Reason,
			"an owned condition is unaffected by the drop")
		assert.Nil(t, meta.FindStatusCondition(persisted.Status.Conditions, "ExternalReady"),
			"an unowned condition the server does not carry must be dropped, not resurrected")
	})

	t.Run("drops a first-time unowned condition the server has never held", func(t *testing.T) {
		// Same rule seen from the other side: an owner-level aggregate the server
		// has never held is not written on the conflict pass. The next reconcile
		// stages it again.
		serverSide := newOwner()
		serverSide.Status.Conditions = []metav1.Condition{{
			Type: "ExternalReady", Status: metav1.ConditionTrue, Reason: "WrittenByOtherController",
			LastTransitionTime: metav1.Now(),
		}}
		k8sClient := conflictingClient(serverSide)

		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "Ready", Status: metav1.ConditionTrue, Reason: "Healthy", // the aggregate, owned by nobody
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, "Healthy", conditionOf(t, persisted, "InfraReady").Reason)
		assert.Equal(t, "WrittenByOtherController", conditionOf(t, persisted, "ExternalReady").Reason)
		assert.Nil(t, meta.FindStatusCondition(persisted.Status.Conditions, "Ready"),
			"an unowned aggregate the server has never held is not written on the conflict pass")
	})

	t.Run("does not fetch at all when the first update succeeds", func(t *testing.T) {
		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})
		inner := fake.NewClientBuilder().
			WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()
		counting := &conflictOnceClient{Client: inner, conflicts: 1} // never conflicts

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: counting, Owner: owner}, []*Component{infraComponent(t)},
		))

		assert.Zero(t, counting.gets, "the non-conflict path must not fetch the owner")
	})
}

// TestFlushStatusOwnedStatusFields covers a status shared between controllers.
// Each case reads the owner, lets a second writer update the status, and then
// flushes, so the first update is a real 409 from a stale resourceVersion.
func TestFlushStatusOwnedStatusFields(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))

	infraComponent := func(t *testing.T) *Component {
		t.Helper()
		comp, err := NewComponentBuilder().WithName("infra").WithConditionType("InfraReady").Build()
		require.NoError(t, err)
		return comp
	}

	// readThenRace creates the owner on a fake server, reads it as the
	// controller would at the top of a reconcile, and then applies the second
	// writer's status update on the server. The returned owner is stale.
	readThenRace := func(
		t *testing.T, initial *MockOperatorCRD, secondWriter func(*MockOperatorCRD),
	) (client.Client, *MockOperatorCRD) {
		t.Helper()
		initial.ObjectMeta = metav1.ObjectMeta{Name: "test-owner", Namespace: "default", Generation: 2}
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).WithStatusSubresource(initial).WithObjects(initial).Build()

		owner := &MockOperatorCRD{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), owner))

		other := &MockOperatorCRD{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), other))
		secondWriter(other)
		require.NoError(t, k8sClient.Status().Update(ctx, other))
		return k8sClient, owner
	}

	persistedOf := func(t *testing.T, c client.Client, owner *MockOperatorCRD) *MockOperatorCRD {
		t.Helper()
		persisted := &MockOperatorCRD{}
		require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(owner), persisted))
		return persisted
	}

	t.Run("keeps a second writer's non-condition field across the conflict retry", func(t *testing.T) {
		k8sClient, owner := readThenRace(t, &MockOperatorCRD{Status: MockStatus{ObservedGeneration: 1}},
			func(o *MockOperatorCRD) { o.Status.ObservedGeneration = 2 })

		// This controller owns phase and InfraReady; observedGeneration belongs
		// to the second writer and is still 1 on the stale owner.
		owner.Status.Phase = "Accepted"
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
			WithOwnedStatusFields("phase"),
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, int64(2), persisted.Status.ObservedGeneration,
			"the retry must not roll back a field this flush does not own")
		assert.Equal(t, "Accepted", persisted.Status.Phase, "an owned field keeps the staged value")
		assert.NotNil(t, meta.FindStatusCondition(persisted.Status.Conditions, "InfraReady"),
			"an owned condition keeps the staged value")
		assert.Equal(t, int64(2), owner.Status.ObservedGeneration,
			"the in-memory owner reflects what was written")
	})

	t.Run("owns no non-condition field when declared with no paths", func(t *testing.T) {
		k8sClient, owner := readThenRace(t, &MockOperatorCRD{Status: MockStatus{Phase: "Pending"}},
			func(o *MockOperatorCRD) {
				o.Status.ObservedGeneration = 2
				o.Status.Phase = "Running"
			})

		owner.Status.ObservedGeneration = 99 // a stray write this flush does not own
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
			WithOwnedStatusFields(),
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, int64(2), persisted.Status.ObservedGeneration)
		assert.Equal(t, "Running", persisted.Status.Phase)
		assert.Equal(t, "Healthy", meta.FindStatusCondition(persisted.Status.Conditions, "InfraReady").Reason)
	})

	t.Run("keeps every staged field when no fields are declared", func(t *testing.T) {
		// Without the option the controller's own staged fields win, so an
		// existing caller that stages observedGeneration does not lose it.
		k8sClient, owner := readThenRace(t, &MockOperatorCRD{},
			func(o *MockOperatorCRD) { o.Status.Phase = "Running" })

		owner.Status.ObservedGeneration = 2

		require.NoError(t, FlushStatus(ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, int64(2), persisted.Status.ObservedGeneration)
		assert.Empty(t, persisted.Status.Phase, "with nothing declared the staged status is written as a whole")
	})

	t.Run("removes an owned field the staged owner cleared", func(t *testing.T) {
		k8sClient, owner := readThenRace(t, &MockOperatorCRD{Status: MockStatus{Phase: "Pending"}},
			func(o *MockOperatorCRD) { o.Status.ObservedGeneration = 2 })

		owner.Status.Phase = ""

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("phase"),
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Empty(t, persisted.Status.Phase, "the staged owner is the source of truth for an owned field")
		assert.Equal(t, int64(2), persisted.Status.ObservedGeneration)
	})

	t.Run("drops an unowned field the server no longer carries", func(t *testing.T) {
		k8sClient, owner := readThenRace(t, &MockOperatorCRD{Status: MockStatus{ObservedGeneration: 1}},
			func(o *MockOperatorCRD) { o.Status.ObservedGeneration = 0 })

		owner.Status.Phase = "Accepted"

		// The REST client decodes a response into the object it is given, so a
		// field the response omits keeps whatever that object held. The fake
		// client replaces the object instead and would hide a stale value.
		require.NoError(t, FlushStatus(
			ctx,
			ReconcileContext{Client: k8sClient, APIReader: decodingReader{Reader: k8sClient}, Owner: owner},
			nil,
			WithOwnedStatusFields("phase"),
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Zero(t, persisted.Status.ObservedGeneration,
			"the server is the source of truth for an unowned field, absence included")
		assert.Equal(t, "Accepted", persisted.Status.Phase)
	})

	t.Run("owns a nested field by its dotted path", func(t *testing.T) {
		k8sClient, owner := readThenRace(t,
			&MockOperatorCRD{Status: MockStatus{Summary: &MockStatusSummary{Ready: 1, Total: 3}}},
			func(o *MockOperatorCRD) { o.Status.Summary.Total = 4 })

		owner.Status.Summary.Ready = 2

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("summary.ready"),
		))

		persisted := persistedOf(t, k8sClient, owner)
		require.NotNil(t, persisted.Status.Summary)
		assert.Equal(t, MockStatusSummary{Ready: 2, Total: 4}, *persisted.Status.Summary)
	})

	t.Run("sets an owned nested field below a parent the server holds as null", func(t *testing.T) {
		k8sClient, owner := readThenRace(t, &MockOperatorCRD{},
			func(o *MockOperatorCRD) { o.Status.ObservedGeneration = 2 })

		owner.Status.Details = &MockStatusSummary{Ready: 1}

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("details.ready"),
		))

		persisted := persistedOf(t, k8sClient, owner)
		require.NotNil(t, persisted.Status.Details)
		assert.Equal(t, int32(1), persisted.Status.Details.Ready)
		assert.Equal(t, int64(2), persisted.Status.ObservedGeneration)
	})

	t.Run("removes an owned nested field below a parent the staged owner holds as null", func(t *testing.T) {
		k8sClient, owner := readThenRace(t,
			&MockOperatorCRD{Status: MockStatus{Details: &MockStatusSummary{Ready: 1}}},
			func(o *MockOperatorCRD) { o.Status.ObservedGeneration = 2 })

		owner.Status.Details = nil

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("details.ready"),
		))

		persisted := persistedOf(t, k8sClient, owner)
		if persisted.Status.Details != nil {
			assert.Zero(t, persisted.Status.Details.Ready)
		}
		assert.Equal(t, int64(2), persisted.Status.ObservedGeneration)
	})

	t.Run("combines the paths of several options", func(t *testing.T) {
		k8sClient, owner := readThenRace(t, &MockOperatorCRD{},
			func(o *MockOperatorCRD) { o.Status.Summary = &MockStatusSummary{Total: 4} })

		owner.Status.Phase = "Accepted"
		owner.Status.ObservedGeneration = 2

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil,
			WithOwnedStatusFields("phase"), WithOwnedStatusFields("observedGeneration"),
		))

		persisted := persistedOf(t, k8sClient, owner)
		assert.Equal(t, "Accepted", persisted.Status.Phase)
		assert.Equal(t, int64(2), persisted.Status.ObservedGeneration)
		require.NotNil(t, persisted.Status.Summary)
		assert.Equal(t, int32(4), persisted.Status.Summary.Total)
	})

	t.Run("keeps owned conditions stored under a JSON name other than conditions", func(t *testing.T) {
		// OperatorCRD reaches the conditions through GetStatusConditions, so
		// their JSON name is the owner type's choice.
		checksScheme := runtime.NewScheme()
		checksScheme.AddKnownTypes(GroupVersion, &checksOwner{})
		initial := &checksOwner{ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: "default"}}
		k8sClient := fake.NewClientBuilder().
			WithScheme(checksScheme).WithStatusSubresource(initial).WithObjects(initial).Build()

		owner := &checksOwner{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), owner))

		other := &checksOwner{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), other))
		other.Status.Phase = "Running"
		require.NoError(t, k8sClient.Status().Update(ctx, other))

		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent(t)},
			WithOwnedStatusFields(),
		))

		persisted := &checksOwner{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), persisted))
		found := meta.FindStatusCondition(persisted.Status.Checks, "InfraReady")
		require.NotNil(t, found, "an owned condition must keep the staged value")
		assert.Equal(t, "Healthy", found.Reason)
		assert.Equal(t, "Running", persisted.Status.Phase)
	})

	t.Run("sets an owned field when the server holds the whole status as null", func(t *testing.T) {
		nullScheme := runtime.NewScheme()
		nullScheme.AddKnownTypes(GroupVersion, &nullableStatusOwner{})
		initial := &nullableStatusOwner{ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: "default"}}
		k8sClient := fake.NewClientBuilder().
			WithScheme(nullScheme).WithStatusSubresource(initial).WithObjects(initial).Build()

		owner := &nullableStatusOwner{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), owner))

		other := &nullableStatusOwner{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), other))
		other.Labels = map[string]string{"touched": "true"}
		require.NoError(t, k8sClient.Update(ctx, other))

		owner.Status = &checksStatus{Phase: "Accepted"}

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("phase"),
		))

		persisted := &nullableStatusOwner{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), persisted))
		require.NotNil(t, persisted.Status)
		assert.Equal(t, "Accepted", persisted.Status.Phase)
	})

	t.Run("keeps a large integer in a dynamic status field exact across the retry", func(t *testing.T) {
		const big = int64(1<<53 + 1) // the first integer a float64 cannot hold

		checksScheme := runtime.NewScheme()
		checksScheme.AddKnownTypes(GroupVersion, &checksOwner{})
		initial := &checksOwner{ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: "default"}}
		k8sClient := fake.NewClientBuilder().
			WithScheme(checksScheme).WithStatusSubresource(initial).WithObjects(initial).Build()

		owner := &checksOwner{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), owner))

		other := &checksOwner{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), other))
		other.Status.Extra = map[string]any{"big": big}
		require.NoError(t, k8sClient.Status().Update(ctx, other))

		owner.Status.Phase = "Accepted"

		require.NoError(t, FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("phase"),
		))

		persisted := &checksOwner{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(initial), persisted))
		assert.Equal(t, big, persisted.Status.Extra["big"])
		assert.Equal(t, "Accepted", persisted.Status.Phase)
	})

	t.Run("rejects a path that the status JSON does not contain", func(t *testing.T) {
		for name, path := range map[string]string{
			"below a type that encodes itself as text": "level.value",
			"unexported field":                         "internal",
			"below a type that encodes itself":         "lastSync.Time",
			"unexported field of such a type":          "lastSync.wall",
		} {
			t.Run(name, func(t *testing.T) {
				owner := &checksOwner{ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: "default"}}
				k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

				err := FlushStatus(
					ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields(path),
				)

				require.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("%q", path))
			})
		}
	})

	t.Run("rejects an invalid path before writing", func(t *testing.T) {
		for name, path := range map[string]string{
			"empty":                "",
			"empty segment":        "summary..ready",
			"conditions":           "conditions",
			"inside conditions":    "conditions.type",
			"unknown field":        "observedGenration",
			"prefixed with status": "status.phase",
			"below a scalar":       "phase.value",
			"unknown nested field": "summary.unknown",
			"trailing separator":   "phase.",
			"leading separator":    ".phase",
		} {
			t.Run(name, func(t *testing.T) {
				owner := &MockOperatorCRD{
					ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: "default"},
				}
				k8sClient := fake.NewClientBuilder().
					WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()
				owner.Status.Phase = "Accepted"

				err := FlushStatus(
					ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields(path),
				)

				require.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("%q", path))
				assert.Empty(t, persistedOf(t, k8sClient, owner).Status.Phase, "nothing is written")
			})
		}
	})
}

// TestJSONFields covers how path validation finds a field through embedded
// structs. More than one candidate makes the path ambiguous, which validation
// rejects rather than guessing which field encoding/json encodes.
func TestJSONFields(t *testing.T) {
	type base struct {
		Phase string `json:"phase"`
	}
	type otherBase struct {
		Phase string `json:"phase"`
	}
	type phaseObject struct {
		Ready bool `json:"ready"`
	}

	t.Run("finds a field of an embedded struct", func(t *testing.T) {
		type status struct {
			base
		}

		assert.Equal(t, []reflect.Type{reflect.TypeOf("")}, jsonFields(reflect.TypeOf(status{}), "phase", nil))
	})

	t.Run("finds a field of a struct embedded by pointer", func(t *testing.T) {
		type status struct {
			*base
		}

		assert.Equal(t, []reflect.Type{reflect.TypeOf("")}, jsonFields(reflect.TypeOf(status{}), "phase", nil))
	})

	// go vet rejects a struct literal that repeats a JSON name, so the
	// ambiguous types are built at run time.
	baseType := reflect.TypeOf(base{})
	embed := func(name string, typ reflect.Type) reflect.StructField {
		return reflect.StructField{Name: name, Type: typ, Anonymous: true}
	}

	t.Run("finds both a declared field and the embedded field it shadows", func(t *testing.T) {
		status := reflect.StructOf([]reflect.StructField{
			embed("Base", baseType),
			{Name: "Phase", Type: reflect.TypeOf(phaseObject{}), Tag: `json:"phase"`},
		})

		assert.Len(t, jsonFields(status, "phase", nil), 2)
	})

	t.Run("finds the field of each embedded struct that provides the name", func(t *testing.T) {
		status := reflect.StructOf([]reflect.StructField{
			embed("Base", baseType),
			embed("OtherBase", reflect.TypeOf(otherBase{})),
		})

		assert.Len(t, jsonFields(status, "phase", nil), 2)
	})

	t.Run("does not recurse forever through a struct that embeds itself", func(t *testing.T) {
		found := jsonFields(reflect.TypeOf(selfEmbedding{}), "phase", nil)

		assert.Equal(t, []reflect.Type{reflect.TypeOf("")}, found)
	})

	t.Run("skips a field tagged to be ignored", func(t *testing.T) {
		type status struct {
			Phase  string `json:"-"`
			Hidden string `json:"phase"`
		}

		assert.Equal(t, []reflect.Type{reflect.TypeOf("")}, jsonFields(reflect.TypeOf(status{}), "phase", nil))
	})
}

// selfEmbedding is valid Go and valid encoding/json input: the embedded
// pointer can stay nil while Phase is encoded.
type selfEmbedding struct {
	*selfEmbedding        //nolint:unused // read only by reflection
	Phase          string `json:"phase"`
}

// checksOwner is an OperatorCRD whose conditions encode as "checks".
type checksOwner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status checksStatus `json:"status,omitempty"`
}

type checksStatus struct {
	Checks   []metav1.Condition `json:"checks,omitempty"`
	Phase    string             `json:"phase,omitempty"`
	LastSync metav1.Time        `json:"lastSync,omitzero"`
	Level    textLevel          `json:"level,omitzero"`
	Extra    map[string]any     `json:"extra,omitempty"`
	internal string             //nolint:unused // read only by reflection
}

// textLevel encodes as a JSON string through MarshalText, so its Value field
// never appears in the JSON.
type textLevel struct {
	Value string `json:"value"`
}

func (l textLevel) MarshalText() ([]byte, error) {
	return []byte(l.Value), nil
}

func (l *textLevel) UnmarshalText(text []byte) error {
	l.Value = string(text)
	return nil
}

func (o *checksOwner) GetStatusConditions() *[]metav1.Condition {
	return &o.Status.Checks
}

func (o *checksOwner) GetKind() string {
	return "checksOwner"
}

func (o *checksOwner) DeepCopyObject() runtime.Object {
	out := *o
	o.ObjectMeta.DeepCopyInto(&out.ObjectMeta) //nolint:staticcheck // naming ObjectMeta keeps the copy readable
	out.Status.Checks = slices.Clone(o.Status.Checks)
	if o.Status.Extra != nil {
		out.Status.Extra = runtime.DeepCopyJSON(o.Status.Extra)
	}
	return &out
}

// nullableStatusOwner encodes a nil status as null, because the field has no
// omitempty.
type nullableStatusOwner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status *checksStatus `json:"status"`
}

func (o *nullableStatusOwner) GetStatusConditions() *[]metav1.Condition {
	if o.Status == nil {
		o.Status = &checksStatus{}
	}
	return &o.Status.Checks
}

func (o *nullableStatusOwner) GetKind() string {
	return "nullableStatusOwner"
}

func (o *nullableStatusOwner) DeepCopyObject() runtime.Object {
	out := *o
	o.ObjectMeta.DeepCopyInto(&out.ObjectMeta) //nolint:staticcheck // naming ObjectMeta keeps the copy readable
	if o.Status != nil {
		status := *o.Status
		status.Checks = slices.Clone(o.Status.Checks)
		if o.Status.Extra != nil {
			status.Extra = runtime.DeepCopyJSON(o.Status.Extra)
		}
		out.Status = &status
	}
	return &out
}

// decodingReader reads like the REST client: it decodes the server's JSON into
// the object passed in, so fields the JSON omits are left as they were.
type decodingReader struct {
	client.Reader
}

func (r decodingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	fresh, ok := obj.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("deep copy of %T is not a client.Object", obj)
	}
	if err := r.Reader.Get(ctx, key, fresh, opts...); err != nil {
		return err
	}

	data, err := json.Marshal(fresh)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, obj)
}
