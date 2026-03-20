package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
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
		mut := feature.Mutation[*mockMutator]{
			Name:    "test-mutation",
			Feature: feature.NewResourceFeature("1.0.0", nil),
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
		tests := []struct {
			name    string
			obj     *corev1.Service
			idFunc  func(*corev1.Service) string
			defApp  FieldApplicator[*corev1.Service]
			newMut  func(*corev1.Service) *mockMutator
			wantErr string
		}{
			{"nil object", nil, identityFunc, defaultApp, newMutator, "object cannot be nil"},
			{"empty name", &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}}, identityFunc, defaultApp, newMutator, "object name cannot be empty"},
			{"empty namespace", &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, identityFunc, defaultApp, newMutator, "object namespace cannot be empty"},
			{"nil identity", obj, nil, defaultApp, newMutator, "identity function cannot be nil"},
			{"nil applicator", obj, identityFunc, nil, newMutator, "default field applicator cannot be nil"},
			{"nil mutator factory", obj, identityFunc, defaultApp, nil, "mutator factory cannot be nil"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := NewIntegrationBuilder(tt.obj, tt.idFunc, tt.defApp, tt.newMut).Build()
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("expected error %q, got %v", tt.wantErr, err)
				}
			})
		}
	})
}
