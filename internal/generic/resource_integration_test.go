package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIntegrationResource(t *testing.T) {
	obj := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
	}
	identityFunc := func(s *corev1.Service) string { return s.Name }
	defaultApp := func(current, desired *corev1.Service) error {
		current.Spec = desired.Spec
		return nil
	}
	newMutator := func(s *corev1.Service) *mockMutator { return &mockMutator{service: s} }

	res := &IntegrationResource[*corev1.Service, *mockMutator]{
		DesiredObject:          obj,
		IdentityFunc:           identityFunc,
		DefaultFieldApplicator: defaultApp,
		NewMutator:             newMutator,
	}

	t.Run("Identity", func(t *testing.T) {
		if res.Identity() != "test-svc" {
			t.Errorf("expected identity test-svc, got %s", res.Identity())
		}
	})

	t.Run("Object", func(t *testing.T) {
		got, err := res.Object()
		if err != nil {
			t.Fatalf("Object() error = %v", err)
		}
		if got.GetName() != "test-svc" {
			t.Errorf("expected name test-svc, got %s", got.GetName())
		}
	})

	t.Run("Mutate", func(t *testing.T) {
		current := &corev1.Service{}
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

	t.Run("Status handlers", func(t *testing.T) {
		res.OperationalStatusHandler = func(_ concepts.ConvergingOperation, _ *corev1.Service) (concepts.OperationalStatusWithReason, error) {
			return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusOperational}, nil
		}
		res.SuspendStatusHandler = func(_ *corev1.Service) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res.DeleteOnSuspendHandler = func(_ *corev1.Service) bool {
			return true
		}

		cs, _ := res.ConvergingStatus(concepts.ConvergingOperationCreated)
		if cs.Status != concepts.OperationalStatusOperational {
			t.Errorf("expected operational")
		}

		ss, _ := res.SuspensionStatus()
		if ss.Status != concepts.SuspensionStatusSuspended {
			t.Errorf("expected suspended")
		}

		if !res.DeleteOnSuspend() {
			t.Errorf("expected delete on suspend true")
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

		current := &corev1.Service{}
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
