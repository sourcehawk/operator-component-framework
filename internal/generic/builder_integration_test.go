//nolint:dupl
package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	newMutator := func(s *corev1.Service) *mockMutator { return &mockMutator{service: s} }

	t.Run("successful build", func(t *testing.T) {
		builder := NewIntegrationBuilder(obj, identityFunc, newMutator)
		res, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, obj, res.DesiredObject)
	})

	t.Run("with mutation", func(t *testing.T) {
		mut := Mutation[*mockMutator]{
			Name:    "test-mutation",
			Feature: alwaysEnabled{},
			Mutate: func(_ *mockMutator) error {
				return nil
			},
		}
		builder := NewIntegrationBuilder(obj, identityFunc, newMutator).WithMutation(mut)
		res, _ := builder.Build()
		assert.Len(t, res.Mutations, 1)
	})

	t.Run("with handlers", func(t *testing.T) {
		builder := NewIntegrationBuilder(obj, identityFunc, newMutator).
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
		assert.NotNil(t, res.OperationalStatusHandler, "OperationalStatusHandler not set")
		assert.NotNil(t, res.SuspendStatusHandler, "SuspendStatusHandler not set")
		assert.NotNil(t, res.SuspendMutationHandler, "SuspendMutationHandler not set")
		assert.NotNil(t, res.DeleteOnSuspendHandler, "DeleteOnSuspendHandler not set")
	})

	t.Run("cluster-scoped build succeeds without namespace", func(t *testing.T) {
		clusterObj := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj"},
		}
		builder := NewIntegrationBuilder(clusterObj, identityFunc, newMutator)
		builder.MarkClusterScoped()
		res, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, clusterObj, res.DesiredObject)
	})

	t.Run("cluster-scoped build rejects non-empty namespace", func(t *testing.T) {
		nsObj := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj", Namespace: "oops"},
		}
		builder := NewIntegrationBuilder(nsObj, identityFunc, newMutator)
		builder.MarkClusterScoped()
		_, err := builder.Build()
		require.EqualError(t, err, errClusterScopedNamespace)
	})

	t.Run("validation errors", func(t *testing.T) {
		runBuilderValidationTests[*IntegrationResource[*corev1.Service, *mockMutator]](
			t, obj, identityFunc, newMutator,
			func(o *corev1.Service, id func(*corev1.Service) string, mut func(*corev1.Service) *mockMutator) genericBuilder[*IntegrationResource[*corev1.Service, *mockMutator]] {
				return NewIntegrationBuilder(o, id, mut)
			},
		)
	})
}
