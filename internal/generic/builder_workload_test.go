package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
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
	defaultApp := func(current, desired *appsv1.Deployment) error { return nil }
	newMutator := func(d *appsv1.Deployment) *mockMutator { return &mockMutator{deployment: d} }

	t.Run("successful build", func(t *testing.T) {
		builder := NewWorkloadBuilder(obj, identityFunc, defaultApp, newMutator)
		res, err := builder.Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if res.Object != obj {
			t.Errorf("expected object %v, got %v", obj, res.Object)
		}
	})

	t.Run("with mutation", func(t *testing.T) {
		mut := feature.Mutation[*mockMutator]{
			Name:    "test-mutation",
			Feature: feature.NewResourceFeature("1.0.0", nil),
			Mutate: func(m *mockMutator) error {
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
			WithCustomConvergeStatus(func(op component.ConvergingOperation, d *appsv1.Deployment) (component.ConvergingStatusWithReason, error) {
				return component.ConvergingStatusWithReason{}, nil
			}).
			WithCustomGraceStatus(func(d *appsv1.Deployment) (component.GraceStatusWithReason, error) {
				return component.GraceStatusWithReason{}, nil
			}).
			WithCustomSuspendStatus(func(d *appsv1.Deployment) (component.SuspensionStatusWithReason, error) {
				return component.SuspensionStatusWithReason{}, nil
			}).
			WithCustomSuspendMutation(func(m *mockMutator) error {
				return nil
			}).
			WithCustomSuspendDeletionDecision(func(d *appsv1.Deployment) bool {
				return true
			})

		res, _ := builder.Build()
		if res.ConvergingStatusHandler == nil || res.GraceStatusHandler == nil || res.SuspendStatusHandler == nil || res.SuspendMutationHandler == nil || res.DeleteOnSuspendHandler == nil {
			t.Errorf("one or more handlers not set")
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		tests := []struct {
			name    string
			obj     *appsv1.Deployment
			idFunc  func(*appsv1.Deployment) string
			defApp  FieldApplicator[*appsv1.Deployment]
			newMut  func(*appsv1.Deployment) *mockMutator
			wantErr string
		}{
			{"nil object", nil, identityFunc, defaultApp, newMutator, "object cannot be nil"},
			{"empty name", &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}}, identityFunc, defaultApp, newMutator, "object name cannot be empty"},
			{"empty namespace", &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, identityFunc, defaultApp, newMutator, "object namespace cannot be empty"},
			{"nil identity", obj, nil, defaultApp, newMutator, "identity function cannot be nil"},
			{"nil applicator", obj, identityFunc, nil, newMutator, "default field applicator cannot be nil"},
			{"nil mutator factory", obj, identityFunc, defaultApp, nil, "mutator factory cannot be nil"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := NewWorkloadBuilder(tt.obj, tt.idFunc, tt.defApp, tt.newMut).Build()
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("expected error %q, got %v", tt.wantErr, err)
				}
			})
		}
	})
}
