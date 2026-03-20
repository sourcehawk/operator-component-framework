package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockTaskMutator struct {
	job     *batchv1.Job
	applied bool
}

func (m *mockTaskMutator) Apply() error {
	m.applied = true
	return nil
}

func TestTaskResource(t *testing.T) {
	obj := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
	}
	identityFunc := func(j *batchv1.Job) string { return j.Name }
	defaultApp := func(current, desired *batchv1.Job) error {
		current.Spec = desired.Spec
		return nil
	}
	newMutator := func(j *batchv1.Job) *mockTaskMutator { return &mockTaskMutator{job: j} }

	res := &TaskResource[*batchv1.Job, *mockTaskMutator]{
		DesiredObject:          obj,
		IdentityFunc:           identityFunc,
		DefaultFieldApplicator: defaultApp,
		NewMutator:             newMutator,
	}

	t.Run("Identity", func(t *testing.T) {
		if res.Identity() != "test-job" {
			t.Errorf("expected identity test-job, got %s", res.Identity())
		}
	})

	t.Run("Object", func(t *testing.T) {
		got, err := res.Object()
		if err != nil {
			t.Fatalf("Object() error = %v", err)
		}
		if got.GetName() != "test-job" {
			t.Errorf("expected name test-job, got %s", got.GetName())
		}
	})

	t.Run("Mutate", func(t *testing.T) {
		current := &batchv1.Job{}
		mutCalled := false
		res.Mutations = []feature.Mutation[*mockTaskMutator]{
			{
				Name:    "test-mut",
				Feature: feature.NewResourceFeature("1.0.0", nil),
				Mutate: func(_ *mockTaskMutator) error {
					mutCalled = true
					return nil
				},
			},
		}

		err := res.Mutate(current)
		if err != nil {
			t.Fatalf("Mutate() error = %v", err)
		}
		if !mutCalled {
			t.Errorf("mutation was not called")
		}
	})

	t.Run("Suspend", func(t *testing.T) {
		suspendMutCalled := false
		res.SuspendMutationHandler = func(_ *mockTaskMutator) error {
			suspendMutCalled = true
			return nil
		}

		err := res.Suspend()
		if err != nil {
			t.Fatalf("Suspend() error = %v", err)
		}

		current := &batchv1.Job{}
		err = res.Mutate(current)
		if err != nil {
			t.Fatalf("Mutate() error = %v", err)
		}

		if !suspendMutCalled {
			t.Errorf("suspend mutation was not called")
		}
		if res.Suspender != nil {
			t.Errorf("suspender should be nil after use")
		}
	})

	t.Run("Status handlers", func(t *testing.T) {
		res.ConvergingStatusHandler = func(_ concepts.ConvergingOperation, _ *batchv1.Job) (concepts.CompletionStatusWithReason, error) {
			return concepts.CompletionStatusWithReason{Status: concepts.CompletionStatusCompleted}, nil
		}
		res.SuspendStatusHandler = func(_ *batchv1.Job) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res.DeleteOnSuspendHandler = func(_ *batchv1.Job) bool {
			return true
		}

		cs, _ := res.ConvergingStatus(concepts.ConvergingOperationCreated)
		if cs.Status != concepts.CompletionStatusCompleted {
			t.Errorf("expected completed")
		}

		ss, _ := res.SuspensionStatus()
		if ss.Status != concepts.SuspensionStatusSuspended {
			t.Errorf("expected suspended")
		}

		if !res.DeleteOnSuspend() {
			t.Errorf("expected delete on suspend true")
		}
	})
}
