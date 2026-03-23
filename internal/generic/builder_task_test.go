//nolint:dupl
package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTaskBuilder(t *testing.T) {
	obj := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
	}
	identityFunc := func(j *batchv1.Job) string { return j.Name }
	defaultApp := func(_, _ *batchv1.Job) error { return nil }
	newMutator := func(j *batchv1.Job) *mockMutator { return &mockMutator{job: j} }

	t.Run("successful build", func(t *testing.T) {
		builder := NewTaskBuilder(obj, identityFunc, defaultApp, newMutator)
		res, err := builder.Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if res.DesiredObject != obj {
			t.Errorf("expected object %v, got %v", obj, res.DesiredObject)
		}
	})

	t.Run("with mutation", func(t *testing.T) {
		mut := Mutation[*mockMutator]{
			Name:    "test-mutation",
			Feature: alwaysEnabled{},
			Mutate: func(_ *mockMutator) error {
				return nil
			},
		}
		builder := NewTaskBuilder(obj, identityFunc, defaultApp, newMutator).WithMutation(mut)
		res, _ := builder.Build()
		if len(res.Mutations) != 1 {
			t.Errorf("expected 1 mutation, got %d", len(res.Mutations))
		}
	})

	t.Run("with handlers", func(t *testing.T) {
		builder := NewTaskBuilder(obj, identityFunc, defaultApp, newMutator).
			WithCustomConvergeStatus(func(_ concepts.ConvergingOperation, _ *batchv1.Job) (concepts.CompletionStatusWithReason, error) {
				return concepts.CompletionStatusWithReason{}, nil
			}).
			WithCustomSuspendStatus(func(_ *batchv1.Job) (concepts.SuspensionStatusWithReason, error) {
				return concepts.SuspensionStatusWithReason{}, nil
			}).
			WithCustomSuspendMutation(func(_ *mockMutator) error {
				return nil
			}).
			WithCustomSuspendDeletionDecision(func(_ *batchv1.Job) bool {
				return true
			})

		res, _ := builder.Build()
		if res.ConvergingStatusHandler == nil || res.SuspendStatusHandler == nil || res.SuspendMutationHandler == nil || res.DeleteOnSuspendHandler == nil {
			t.Errorf("one or more handlers not set")
		}
	})

	t.Run("cluster-scoped build succeeds without namespace", func(t *testing.T) {
		clusterObj := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj"},
		}
		builder := NewTaskBuilder(clusterObj, identityFunc, defaultApp, newMutator)
		builder.MarkClusterScoped()
		res, err := builder.Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if res.DesiredObject != clusterObj {
			t.Errorf("expected object %v, got %v", clusterObj, res.DesiredObject)
		}
	})

	t.Run("cluster-scoped build rejects non-empty namespace", func(t *testing.T) {
		nsObj := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj", Namespace: "oops"},
		}
		builder := NewTaskBuilder(nsObj, identityFunc, defaultApp, newMutator)
		builder.MarkClusterScoped()
		_, err := builder.Build()
		if err == nil || err.Error() != errClusterScopedNamespace {
			t.Errorf("expected cluster-scoped namespace error, got %v", err)
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		runBuilderValidationTests[*TaskResource[*batchv1.Job, *mockMutator]](
			t, obj, identityFunc, defaultApp, newMutator,
			func(o *batchv1.Job, id func(*batchv1.Job) string, app FieldApplicator[*batchv1.Job], mut func(*batchv1.Job) *mockMutator) genericBuilder[*TaskResource[*batchv1.Job, *mockMutator]] {
				return NewTaskBuilder(o, id, app, mut)
			},
		)
	})
}
