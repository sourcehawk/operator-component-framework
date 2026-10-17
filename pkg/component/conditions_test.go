package component

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
