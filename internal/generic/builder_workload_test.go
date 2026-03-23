package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkloadBuilder(t *testing.T) {
	obj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "default",
		},
	}
	identityFunc := func(d *appsv1.Deployment) string { return d.Name }
	defaultApp := func(_, _ *appsv1.Deployment) error { return nil }
	newMutator := func(d *appsv1.Deployment) *mockMutator { return &mockMutator{deployment: d} }

	t.Run("successful build", func(t *testing.T) {
		builder := NewWorkloadBuilder(obj, identityFunc, defaultApp, newMutator)
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
		builder := NewWorkloadBuilder(obj, identityFunc, defaultApp, newMutator).WithMutation(mut)
		res, _ := builder.Build()
		if len(res.Mutations) != 1 {
			t.Errorf("expected 1 mutation, got %d", len(res.Mutations))
		}
	})

	t.Run("with handlers", func(t *testing.T) {
		builder := NewWorkloadBuilder(obj, identityFunc, defaultApp, newMutator).
			WithCustomConvergeStatus(func(_ concepts.ConvergingOperation, _ *appsv1.Deployment) (concepts.AliveStatusWithReason, error) {
				return concepts.AliveStatusWithReason{}, nil
			}).
			WithCustomGraceStatus(func(_ *appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
				return concepts.GraceStatusWithReason{}, nil
			}).
			WithCustomSuspendStatus(func(_ *appsv1.Deployment) (concepts.SuspensionStatusWithReason, error) {
				return concepts.SuspensionStatusWithReason{}, nil
			}).
			WithCustomSuspendMutation(func(_ *mockMutator) error {
				return nil
			}).
			WithCustomSuspendDeletionDecision(func(_ *appsv1.Deployment) bool {
				return true
			})

		res, _ := builder.Build()
		if res.ConvergingStatusHandler == nil || res.GraceStatusHandler == nil || res.SuspendStatusHandler == nil || res.SuspendMutationHandler == nil || res.DeleteOnSuspendHandler == nil {
			t.Errorf("one or more handlers not set")
		}
	})

	t.Run("cluster-scoped build succeeds without namespace", func(t *testing.T) {
		clusterObj := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj"},
		}
		builder := NewWorkloadBuilder(clusterObj, identityFunc, defaultApp, newMutator)
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
		nsObj := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj", Namespace: "oops"},
		}
		builder := NewWorkloadBuilder(nsObj, identityFunc, defaultApp, newMutator)
		builder.MarkClusterScoped()
		_, err := builder.Build()
		if err == nil || err.Error() != errClusterScopedNamespace {
			t.Errorf("expected cluster-scoped namespace error, got %v", err)
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		runBuilderValidationTests[*WorkloadResource[*appsv1.Deployment, *mockMutator]](
			t, obj, identityFunc, defaultApp, newMutator,
			func(o *appsv1.Deployment, id func(*appsv1.Deployment) string, app FieldApplicator[*appsv1.Deployment], mut func(*appsv1.Deployment) *mockMutator) genericBuilder[*WorkloadResource[*appsv1.Deployment, *mockMutator]] {
				return NewWorkloadBuilder(o, id, app, mut)
			},
		)
	})
}
