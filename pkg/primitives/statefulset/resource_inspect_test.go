package statefulset

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResource_MutationInspector(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sts", Namespace: "test-ns"},
	}
	res, err := NewBuilder(sts).
		WithMutation(Mutation{Name: "Always"}).
		Build()
	require.NoError(t, err)

	var insp concepts.MutationInspector = res
	assert.Equal(t, []string{"Always"}, insp.RegisteredMutations())

	firing, err := insp.FiringSet()
	require.NoError(t, err)
	assert.Equal(t, []string{"Always"}, firing)
}

func TestBuild_RejectsDuplicateMutationNames(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sts", Namespace: "test-ns"},
	}
	_, err := NewBuilder(sts).
		WithMutation(Mutation{Name: "ContainerImage"}).
		WithMutation(Mutation{Name: "ContainerImage"}).
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate mutation name")
	assert.Contains(t, err.Error(), "ContainerImage")
}
