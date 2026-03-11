package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
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
		Object:                 obj,
		IdentityFunc:           identityFunc,
		DefaultFieldApplicator: defaultApp,
		NewMutator:             newMutator,
	}

	t.Run("Identity", func(t *testing.T) {
		if res.Identity() != "test-deploy" {
			t.Errorf("expected identity test-deploy, got %s", res.Identity())
		}
	})

	t.Run("GetObject", func(t *testing.T) {
		got, err := res.GetObject()
		if err != nil {
			t.Fatalf("GetObject() error = %v", err)
		}
		if got.GetName() != "test-deploy" {
			t.Errorf("expected name test-deploy, got %s", got.GetName())
		}
	})

	t.Run("Mutate", func(t *testing.T) {
		current := &appsv1.Deployment{}
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
		res.ConvergingStatusHandler = func(_ component.ConvergingOperation, _ *appsv1.Deployment) (component.ConvergingStatusWithReason, error) {
			return component.ConvergingStatusWithReason{Status: component.ConvergingStatusReady}, nil
		}
		res.GraceStatusHandler = func(_ *appsv1.Deployment) (component.GraceStatusWithReason, error) {
			return component.GraceStatusWithReason{Status: component.GraceStatusReady}, nil
		}
		res.SuspendStatusHandler = func(_ *appsv1.Deployment) (component.SuspensionStatusWithReason, error) {
			return component.SuspensionStatusWithReason{Status: component.SuspensionStatusSuspended}, nil
		}
		res.DeleteOnSuspendHandler = func(_ *appsv1.Deployment) bool {
			return true
		}

		cs, _ := res.ConvergingStatus(component.ConvergingOperationCreated)
		if cs.Status != component.ConvergingStatusReady {
			t.Errorf("expected ready")
		}

		gs, _ := res.GraceStatus()
		if gs.Status != component.GraceStatusReady {
			t.Errorf("expected ready")
		}

		ss, _ := res.SuspensionStatus()
		if ss.Status != component.SuspensionStatusSuspended {
			t.Errorf("expected suspended")
		}

		if !res.DeleteOnSuspend() {
			t.Errorf("expected delete on suspend true")
		}
	})
}
