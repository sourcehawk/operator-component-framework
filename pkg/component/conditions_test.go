package component

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	t.Run("Ready status", func(t *testing.T) {
		converging := ConvergingStatusWithReason{
			Status: ConvergingStatusReady,
			Reason: "All resources ready.",
		}
		cond := convergingCondition(componentType, converging, observedGen)

		assert.Equal(t, string(componentType), cond.Type)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Ready), cond.Reason)
		assert.Equal(t, converging.Reason, cond.Message)
		assert.Equal(t, observedGen, cond.ObservedGeneration)
	})

	t.Run("Creating status", func(t *testing.T) {
		converging := ConvergingStatusWithReason{
			Status: ConvergingStatusCreating,
			Reason: "Resource is creating",
		}
		cond := convergingCondition(componentType, converging, observedGen)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(Creating), cond.Reason)
		assert.Equal(t, converging.Reason, cond.Message)
	})
}

func TestSuspendingCondition(t *testing.T) {
	componentType := ConditionType("TestComponent")
	observedGen := int64(1)

	t.Run("Suspended status", func(t *testing.T) {
		suspending := SuspensionStatusWithReason{
			Status: SuspensionStatusSuspended,
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
		suspending := SuspensionStatusWithReason{
			Status: SuspensionStatusSuspending,
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
		gracing := GraceStatusWithReason{
			Status: GraceStatusDegraded,
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
		gracing := GraceStatusWithReason{
			Status: GraceStatusDown,
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
		assert.Equal(t, string(Ready), cond.Reason)
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
}

func TestConditionMethods(t *testing.T) {
	cond := Condition{
		Type:   "TestComponent",
		Reason: string(Ready),
		Status: metav1.ConditionTrue,
	}

	t.Run("ConditionType", func(t *testing.T) {
		assert.Equal(t, ConditionType("TestComponent"), cond.ConditionType())
	})

	t.Run("ComponentStatus", func(t *testing.T) {
		assert.Equal(t, Ready, cond.ComponentStatus())
	})
}

func TestSetStatusCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)

	owner := &MockOperatorCRD{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-owner",
			Namespace:  "default",
			Generation: 1,
		},
	}

	ctx := context.Background()
	cond := Condition{
		Type:               "TestComponent",
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		Message:            "Ready to go",
		ObservedGeneration: 1,
	}

	t.Run("should update status and record metrics", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()
		metrics := &MockMetrics{}

		rec := ReconcileContext{
			Client:  k8sClient,
			Metrics: metrics,
			Owner:   owner,
		}

		metrics.On("RecordConditionFor",
			owner.GetKind(), owner, cond.Type, string(cond.Status),
			cond.Reason, mock.AnythingOfType("time.Time"), mock.Anything,
		).Return()

		err := setStatusCondition(ctx, rec, cond)
		require.NoError(t, err)

		// Verify condition set in owner
		updated := owner.GetStatusConditions()
		assert.Len(t, *updated, 1)
		assert.Equal(t, cond.Type, (*updated)[0].Type)

		metrics.AssertExpectations(t)
	})

	t.Run("should return error if update fails", func(t *testing.T) {
		// Use a manual mock client to simulate error
		innerClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()
		k8sClient := &errorMockClient{Client: innerClient}
		metrics := &MockMetrics{}

		rec := ReconcileContext{
			Client:  k8sClient,
			Metrics: metrics,
			Owner:   owner,
		}

		metrics.On("RecordConditionFor",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
		).Return()

		// New condition to ensure changed=true
		newCond := cond
		newCond.Message = "Something changed"

		err := setStatusCondition(ctx, rec, newCond)
		require.Error(t, err)
		assert.Equal(t, "update failed", err.Error())
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
