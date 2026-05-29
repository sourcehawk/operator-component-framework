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
