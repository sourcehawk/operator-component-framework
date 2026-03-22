package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkloadResource(t *testing.T) {
	obj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "default",
		},
	}
	identityFunc := func(d *appsv1.Deployment) string { return d.Name }
	defaultApp := func(current, desired *appsv1.Deployment) error {
		current.Spec = desired.Spec
		return nil
	}
	newMutator := func(d *appsv1.Deployment) *mockMutator { return &mockMutator{deployment: d} }

	res := &WorkloadResource[*appsv1.Deployment, *mockMutator]{
		BaseResource: BaseResource[*appsv1.Deployment, *mockMutator]{
			DesiredObject:          obj,
			IdentityFunc:           identityFunc,
			DefaultFieldApplicator: defaultApp,
			NewMutator:             newMutator,
		},
	}

	t.Run("Identity", func(t *testing.T) {
		if res.Identity() != "test-deploy" {
			t.Errorf("expected identity test-deploy, got %s", res.Identity())
		}
	})

	t.Run("Object", func(t *testing.T) {
		got, err := res.Object()
		if err != nil {
			t.Fatalf("Object() error = %v", err)
		}
		if got.GetName() != "test-deploy" {
			t.Errorf("expected name test-deploy, got %s", got.GetName())
		}
	})

	t.Run("Mutate", func(t *testing.T) {
		current := &appsv1.Deployment{}
		mutCalled := false
		res.Mutations = []Mutation[*mockMutator]{
			{
				Name:    "test-mut",
				Feature: alwaysEnabled{},
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
	})

	t.Run("Suspend", func(t *testing.T) {
		suspendMutCalled := false
		res.SuspendMutationHandler = func(_ *mockMutator) error {
			suspendMutCalled = true
			return nil
		}

		err := res.Suspend()
		if err != nil {
			t.Fatalf("Suspend() error = %v", err)
		}

		current := &appsv1.Deployment{}
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
		res.ConvergingStatusHandler = func(_ concepts.ConvergingOperation, _ *appsv1.Deployment) (concepts.AliveStatusWithReason, error) {
			return concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusHealthy}, nil
		}
		res.GraceStatusHandler = func(_ *appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
			return concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}, nil
		}
		res.SuspendStatusHandler = func(_ *appsv1.Deployment) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res.DeleteOnSuspendHandler = func(_ *appsv1.Deployment) bool {
			return true
		}

		cs, _ := res.ConvergingStatus(concepts.ConvergingOperationCreated)
		if cs.Status != concepts.AliveConvergingStatusHealthy {
			t.Errorf("expected healthy")
		}

		gs, _ := res.GraceStatus()
		if gs.Status != concepts.GraceStatusHealthy {
			t.Errorf("expected healthy")
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
