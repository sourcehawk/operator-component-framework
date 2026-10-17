package component

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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

	t.Run("skips metrics when the recorder is nil", func(t *testing.T) {
		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, cond("InfraReady", "Ready", metav1.ConditionTrue))

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()

		require.NoError(t, FlushStatus(ctx, ReconcileContext{Client: k8sClient, Owner: owner}))

		persisted := &MockOperatorCRD{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), persisted))
		assert.Len(t, persisted.Status.Conditions, 1)
	})

	t.Run("returns a non-conflict update error without recording metrics", func(t *testing.T) {
		owner := newOwner()
		applyStatusCondition(ReconcileContext{Owner: owner}, cond("InfraReady", "Ready", metav1.ConditionTrue))

		inner := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(owner).WithObjects(owner).Build()
		k8sClient := &errorMockClient{Client: inner}
		metrics := &MockMetrics{}

		err := FlushStatus(ctx, ReconcileContext{Client: k8sClient, Metrics: metrics, Owner: owner})
		require.ErrorIs(t, err, errUpdateFailed)
		assert.False(t, apierrors.IsConflict(err))

		metrics.AssertNotCalled(t, "RecordConditionFor")
	})
}

var errUpdateFailed = errors.New("update failed")

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
	return errUpdateFailed
}

// Two controllers share the status of one owner: this one owns the Accepted
// condition, another owns Ready and status.observedGeneration. The specs run
// against a real API server because the guarantee rests on its
// resourceVersion check.
var _ = Describe("FlushStatus on an owner whose status has another writer", func() {
	var (
		ctx   = context.Background()
		owner *MockOperatorCRD
	)

	fetch := func() *MockOperatorCRD {
		GinkgoHelper()
		fetched := &MockOperatorCRD{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), fetched)).To(Succeed())
		return fetched
	}

	// otherWriterUpdates writes the other controller's fields through its own
	// fresh read, as a second controller would.
	otherWriterUpdates := func() {
		GinkgoHelper()
		other := fetch()
		other.Status.ObservedGeneration = 4
		meta.SetStatusCondition(&other.Status.Conditions, metav1.Condition{
			Type: "Ready", Status: metav1.ConditionTrue, Reason: "PluginReady",
		})
		Expect(k8sClient.Status().Update(ctx, other)).To(Succeed())
	}

	stageAccepted := func(o *MockOperatorCRD) {
		applyStatusCondition(ReconcileContext{Owner: o}, Condition{
			Type: "Accepted", Status: metav1.ConditionTrue, Reason: "Valid",
		})
	}

	BeforeEach(func() {
		owner = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "shared-status-owner",
				Namespace: createNamespace(ctx, "flush-status-"),
			},
		}
		Expect(k8sClient.Create(ctx, owner)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, owner)).To(Succeed())
	})

	It("returns a conflict and writes nothing when the other writer changed the status after the read", func() {
		stale := fetch()
		otherWriterUpdates()
		stageAccepted(stale)
		metrics := &MockMetrics{}

		err := FlushStatus(ctx, ReconcileContext{Client: k8sClient, Metrics: metrics, Owner: stale})

		Expect(apierrors.IsConflict(err)).To(BeTrue(), "expected a conflict, got %v", err)
		server := fetch()
		Expect(server.Status.ObservedGeneration).To(Equal(int64(4)),
			"the other writer's observedGeneration must not be rolled back")
		ready := meta.FindStatusCondition(server.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Reason).To(Equal("PluginReady"))
		Expect(meta.FindStatusCondition(server.Status.Conditions, "Accepted")).To(BeNil(),
			"nothing from the stale write may land")
		Expect(metrics.Calls).To(BeEmpty(), "a failed write records no condition metrics")
	})

	It("writes its own change and keeps the other writer's fields on the next reconcile", func() {
		stale := fetch()
		otherWriterUpdates()
		stageAccepted(stale)
		Expect(apierrors.IsConflict(FlushStatus(ctx, ReconcileContext{Client: k8sClient, Owner: stale}))).To(BeTrue())

		fresh := fetch()
		stageAccepted(fresh)
		Expect(FlushStatus(ctx, ReconcileContext{Client: k8sClient, Owner: fresh})).To(Succeed())

		server := fetch()
		Expect(server.Status.ObservedGeneration).To(Equal(int64(4)))
		ready := meta.FindStatusCondition(server.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Reason).To(Equal("PluginReady"))
		accepted := meta.FindStatusCondition(server.Status.Conditions, "Accepted")
		Expect(accepted).NotTo(BeNil())
		Expect(accepted.Reason).To(Equal("Valid"))
	})
})
