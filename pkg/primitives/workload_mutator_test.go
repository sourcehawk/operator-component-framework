package primitives_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// emitShared is a single workload-kind-agnostic emitter used for both kinds.
func emitShared() feature.Mutation[primitives.WorkloadMutator] {
	return feature.Mutation[primitives.WorkloadMutator]{
		Name: "shared-env",
		Mutate: func(m primitives.WorkloadMutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}
}

func TestWorkloadMutator_OneEmitterTwoKinds(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
			},
		},
	}
	sm := statefulset.NewMutator(sts)
	require.NoError(t, statefulset.LiftMutation(emitShared()).Mutate(sm))
	require.NoError(t, sm.Apply())
	require.Len(t, sts.Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, "SHARED", sts.Spec.Template.Spec.Containers[0].Env[0].Name)

	dep := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
			},
		},
	}
	dm := deployment.NewMutator(dep)
	require.NoError(t, deployment.LiftMutation(emitShared()).Mutate(dm))
	require.NoError(t, dm.Apply())
	require.Len(t, dep.Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, "SHARED", dep.Spec.Template.Spec.Containers[0].Env[0].Name)
}
