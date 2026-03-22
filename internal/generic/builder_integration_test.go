package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIntegrationBuilder(t *testing.T) {
	obj := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
	}
	identityFunc := func(s *corev1.Service) string { return s.Name }
	defaultApp := func(_, _ *corev1.Service) error { return nil }
	newMutator := func(s *corev1.Service) *mockMutator { return &mockMutator{service: s} }

	t.Run("successful build", func(t *testing.T) {
		builder := NewIntegrationBuilder(obj, identityFunc, defaultApp, newMutator)
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
		builder := NewIntegrationBuilder(obj, identityFunc, defaultApp, newMutator).WithMutation(mut)
		res, _ := builder.Build()
		if len(res.Mutations) != 1 {
			t.Errorf("expected 1 mutation, got %d", len(res.Mutations))
		}
	})

	t.Run("with handlers", func(t *testing.T) {
		builder := NewIntegrationBuilder(obj, identityFunc, defaultApp, newMutator).
			WithCustomOperationalStatus(func(_ concepts.ConvergingOperation, _ *corev1.Service) (concepts.OperationalStatusWithReason, error) {
				return concepts.OperationalStatusWithReason{}, nil
			}).
			WithCustomSuspendStatus(func(_ *corev1.Service) (concepts.SuspensionStatusWithReason, error) {
				return concepts.SuspensionStatusWithReason{}, nil
			}).
			WithCustomSuspendMutation(func(_ *mockMutator) error {
				return nil
			}).
			WithCustomSuspendDeletionDecision(func(_ *corev1.Service) bool {
				return true
			})

		res, _ := builder.Build()
		if res.OperationalStatusHandler == nil || res.SuspendStatusHandler == nil || res.SuspendMutationHandler == nil || res.DeleteOnSuspendHandler == nil {
			t.Errorf("one or more handlers not set")
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		runBuilderValidationTests[*IntegrationResource[*corev1.Service, *mockMutator]](
			t, obj, identityFunc, defaultApp, newMutator,
			func(o *corev1.Service, id func(*corev1.Service) string, app FieldApplicator[*corev1.Service], mut func(*corev1.Service) *mockMutator) genericBuilder[*IntegrationResource[*corev1.Service, *mockMutator]] {
				return NewIntegrationBuilder(o, id, app, mut)
			},
		)
	})
}
