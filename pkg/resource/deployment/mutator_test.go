package deployment

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestMutator_EnvVars(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Env: []corev1.EnvVar{
								{Name: "KEEP", Value: "stay"},
								{Name: "CHANGE", Value: "old"},
								{Name: "REMOVE", Value: "gone"},
							},
						},
					},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CHANGE", Value: "new"})
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "ADD", Value: "added"})
	m.RemoveContainerEnvVars([]string{"REMOVE", "NONEXISTENT"})

	err := m.Apply()
	require.NoError(t, err)

	env := deploy.Spec.Template.Spec.Containers[0].Env
	assert.Len(t, env, 3)

	findEnv := func(name string) *corev1.EnvVar {
		for _, e := range env {
			if e.Name == name {
				return &e
			}
		}
		return nil
	}

	assert.NotNil(t, findEnv("KEEP"))
	assert.Equal(t, "stay", findEnv("KEEP").Value)
	assert.Equal(t, "new", findEnv("CHANGE").Value)
	assert.Equal(t, "added", findEnv("ADD").Value)
	assert.Nil(t, findEnv("REMOVE"))
}

func TestMutator_Args(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Args: []string{"--keep", "--change=old", "--remove"},
						},
					},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.EnsureContainerArg("--change=new")
	m.EnsureContainerArg("--add")
	m.RemoveContainerArgs([]string{"--remove", "--nonexistent"})

	err := m.Apply()
	require.NoError(t, err)

	args := deploy.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--keep")
	assert.Contains(t, args, "--change=old")
	assert.Contains(t, args, "--change=new")
	assert.Contains(t, args, "--add")
	assert.NotContains(t, args, "--remove")
}

func TestMutator_Replicas(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
		},
	}

	m := NewMutator(deploy)
	m.EnsureReplicas(5)

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, int32(5), *deploy.Spec.Replicas)
}

func TestNewMutator(t *testing.T) {
	deploy := &appsv1.Deployment{}
	m := NewMutator(deploy)
	assert.NotNil(t, m)
	assert.Equal(t, deploy, m.current)
}

func TestMutator_EditContainers(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "c1"},
						{Name: "c2"},
					},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.EditContainers(selectors.ContainerNamed("c1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "c1-image"
		return nil
	})
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.EnsureEnvVar(corev1.EnvVar{Name: "GLOBAL", Value: "true"})
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, "c1-image", deploy.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "", deploy.Spec.Template.Spec.Containers[1].Image)
	assert.Equal(t, "GLOBAL", deploy.Spec.Template.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "GLOBAL", deploy.Spec.Template.Spec.Containers[1].Env[0].Name)
}

func TestMutator_EditPodSpec(t *testing.T) {
	deploy := &appsv1.Deployment{}
	m := NewMutator(deploy)
	m.EditPodSpec(func(e *editors.PodSpecEditor) error {
		e.Raw().ServiceAccountName = "my-sa"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "my-sa", deploy.Spec.Template.Spec.ServiceAccountName)
}

func TestMutator_EditDeploymentSpec(t *testing.T) {
	deploy := &appsv1.Deployment{}
	m := NewMutator(deploy)
	m.EditDeploymentSpec(func(e *editors.DeploymentSpecEditor) error {
		e.SetPaused(true)
		e.SetMinReadySeconds(10)
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.True(t, deploy.Spec.Paused)
	assert.Equal(t, int32(10), deploy.Spec.MinReadySeconds)
}

func TestMutator_EditMetadata(t *testing.T) {
	deploy := &appsv1.Deployment{}
	m := NewMutator(deploy)
	m.EditDeploymentMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Labels = map[string]string{"deploy": "label"}
		return nil
	})
	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Annotations = map[string]string{"pod": "ann"}
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "label", deploy.Labels["deploy"])
	assert.Equal(t, "ann", deploy.Spec.Template.Annotations["pod"])
}

func TestMutator_Errors(t *testing.T) {
	deploy := &appsv1.Deployment{}
	m := NewMutator(deploy)
	m.EditPodSpec(func(e *editors.PodSpecEditor) error {
		return errors.New("boom")
	})

	err := m.Apply()
	assert.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestMutator_Order(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"orig": "label"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}

	var order []string

	m := NewMutator(deploy)
	// 6. Container edits
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		order = append(order, "container")
		return nil
	})
	// 5. Pod spec edits
	m.EditPodSpec(func(e *editors.PodSpecEditor) error {
		order = append(order, "podspec")
		return nil
	})
	// 4. Pod template metadata edits
	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
		order = append(order, "podmeta")
		return nil
	})
	// 3. Deployment spec edits
	m.EditDeploymentSpec(func(e *editors.DeploymentSpecEditor) error {
		order = append(order, "depspec")
		return nil
	})
	// 2. Deployment metadata edits
	m.EditDeploymentMetadata(func(e *editors.ObjectMetaEditor) error {
		order = append(order, "depmeta")
		return nil
	})
	// 1. Replicas (now via depspec)
	m.EnsureReplicas(3)

	err := m.Apply()
	require.NoError(t, err)

	// Verify order: depmeta -> depspec -> podmeta -> podspec -> container
	// Replicas is also a depspec edit, so it will trigger the depspec callback if we had another one,
	// but here we just check the sequence of callbacks.
	expected := []string{"depmeta", "depspec", "podmeta", "podspec", "container"}
	assert.Equal(t, expected, order)
	assert.Equal(t, int32(3), *deploy.Spec.Replicas)
}

func TestMutator_NilSafety(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
	m := NewMutator(deploy)

	// These should all be no-ops and not panic
	m.EditContainers(nil, func(e *editors.ContainerEditor) error { return nil })
	m.EditContainers(selectors.AllContainers(), nil)
	m.EditPodSpec(nil)
	m.EditPodTemplateMetadata(nil)
	m.EditDeploymentMetadata(nil)
	m.EditDeploymentSpec(nil)

	err := m.Apply()
	assert.NoError(t, err)
}
