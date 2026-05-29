package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestBaseResourcePreviewReturnsClientObject(t *testing.T) {
	base := &BaseResource[*appsv1.Deployment, *mockMutator]{
		DesiredObject: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		},
		IdentityFunc: func(d *appsv1.Deployment) string { return d.Name },
		NewMutator:   func(d *appsv1.Deployment) *mockMutator { return &mockMutator{deployment: d} },
	}

	var obj client.Object
	obj, err := base.Preview()
	require.NoError(t, err)

	dep, ok := obj.(*appsv1.Deployment)
	require.True(t, ok)
	assert.Equal(t, "web", dep.Name)

	// Preview must not mutate internal state: Object() still returns the baseline.
	baseline, err := base.Object()
	require.NoError(t, err)
	assert.Equal(t, "web", baseline.(*appsv1.Deployment).Name)
}

// TestBaseResourcePreviewDoesNotMutateInternalState verifies that Preview() applies
// registered mutations to the returned object without modifying the resource's
// internal DesiredObject.
func TestBaseResourcePreviewDoesNotMutateInternalState(t *testing.T) {
	base := &BaseResource[*appsv1.Deployment, *mockMutator]{
		DesiredObject: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			// No labels on the baseline.
		},
		IdentityFunc: func(d *appsv1.Deployment) string { return d.Name },
		NewMutator:   func(d *appsv1.Deployment) *mockMutator { return &mockMutator{deployment: d} },
		Mutations: []Mutation[*mockMutator]{
			mockMutation(func(m *mockMutator) error {
				if m.deployment.Labels == nil {
					m.deployment.Labels = map[string]string{}
				}
				m.deployment.Labels["mutated"] = "true"
				return nil
			}),
		},
	}

	obj, err := base.Preview()
	require.NoError(t, err)

	dep, ok := obj.(*appsv1.Deployment)
	require.True(t, ok)

	// The returned object must have the mutation applied.
	assert.Equal(t, "true", dep.Labels["mutated"], "mutation must be present on the previewed object")

	// The internal DesiredObject must be unchanged.
	assert.Empty(t, base.DesiredObject.Labels, "Preview must not modify internal DesiredObject")

	// Object() deep-copies DesiredObject; the copy must also be clean.
	baseline, err := base.Object()
	require.NoError(t, err)
	assert.Empty(t, baseline.(*appsv1.Deployment).Labels, "Object() must reflect unmodified DesiredObject")
}
