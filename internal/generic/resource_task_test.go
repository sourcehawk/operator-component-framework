//nolint:dupl
package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	newMutator := func(j *batchv1.Job) *mockMutator { return &mockMutator{job: j} }

	res := &TaskResource[*batchv1.Job, *mockMutator]{
		BaseResource: BaseResource[*batchv1.Job, *mockMutator]{
			DesiredObject:          obj,
			IdentityFunc:           identityFunc,
			DefaultFieldApplicator: defaultApp,
			NewMutator:             newMutator,
		},
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

	t.Run("Mutate and Suspend", func(t *testing.T) {
		current := &batchv1.Job{}
		mutCalled := false
		res.Mutations = []feature.Mutation[*mockMutator]{
			{
				Name:    "test-mut",
				Feature: feature.NewResourceFeature("1.0.0", nil),
				Mutate: func(_ *mockMutator) error {
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

		suspendMutCalled := false
		res.SuspendMutationHandler = func(_ *mockMutator) error {
			suspendMutCalled = true
			return nil
		}

		err = res.Suspend()
		if err != nil {
			t.Fatalf("Suspend() error = %v", err)
		}

		current = &batchv1.Job{}
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
}
