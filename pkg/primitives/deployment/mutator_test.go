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
	m.BeginFeature()
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CHANGE", Value: "new"})
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "ADD", Value: "added"})
	m.RemoveContainerEnvVars([]string{"REMOVE", "NONEXISTENT"})

	err := m.Apply()
	require.NoError(t, err)

	env := deploy.Spec.Template.Spec.Containers[0].Env
	assert.Len(t, env, 3)

	findEnv := func(name string) *corev1.EnvVar {
		for i := range env {
			if env[i].Name == name {
				return &env[i]
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
	m.BeginFeature()
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
	m.BeginFeature()
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
	assert.Empty(t, m.plans, "NewMutator must not create any plans")
	assert.Nil(t, m.active, "active plan must not be set")
}

func TestBeginFeature_AddsExactlyOnePlan(t *testing.T) {
	deploy := &appsv1.Deployment{}
	m := NewMutator(deploy)

	m.BeginFeature()
	require.Len(t, m.plans, 1, "BeginFeature must add exactly one plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the new plan")

	m.BeginFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestBeginFeature_IsolatesFeaturePlans(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}
	m := NewMutator(deploy)

	// Record mutations in the first feature plan
	m.BeginFeature()
	m.EnsureReplicas(3)
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v1"
		return nil
	})

	// Start a new feature and record different mutations
	m.BeginFeature()
	m.EnsureReplicas(5)

	// First plan should have its edits, second plan should have its own
	assert.Len(t, m.plans[0].deploymentSpecEdits, 1, "first plan should have one spec edit")
	assert.Len(t, m.plans[0].containerEdits, 1, "first plan should have one container edit")
	assert.Len(t, m.plans[1].deploymentSpecEdits, 1, "second plan should have one spec edit")
	assert.Empty(t, m.plans[1].containerEdits, "second plan should have no container edits")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	deploy := &appsv1.Deployment{}
	m := NewMutator(deploy)
	m.BeginFeature()
	m.EnsureReplicas(3)

	require.NoError(t, m.Apply())
	assert.Len(t, m.plans, 1, "no extra plans should be created during Apply")
	assert.Equal(t, int32(3), *deploy.Spec.Replicas)
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
	m.BeginFeature()
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
	m.BeginFeature()
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
	m.BeginFeature()
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
	m.BeginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
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
	m.BeginFeature()
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
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
	m.BeginFeature()
	// 6. Container edits
	m.EditContainers(selectors.AllContainers(), func(_ *editors.ContainerEditor) error {
		order = append(order, "container")
		return nil
	})
	// 5. Pod spec edits
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		order = append(order, "podspec")
		return nil
	})
	// 4. Pod template metadata edits
	m.EditPodTemplateMetadata(func(_ *editors.ObjectMetaEditor) error {
		order = append(order, "podmeta")
		return nil
	})
	// 3. Deployment spec edits
	m.EditDeploymentSpec(func(_ *editors.DeploymentSpecEditor) error {
		order = append(order, "depspec")
		return nil
	})
	// 2. Deployment metadata edits
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
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

func TestMutator_InitContainers(t *testing.T) {
	const newImage = "new-image"
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "old-image"},
					},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.BeginFeature()
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = newImage
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, newImage, deploy.Spec.Template.Spec.InitContainers[0].Image)
}

func TestMutator_ContainerPresence(t *testing.T) {
	const newImage = "new-image"
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app-image"},
						{Name: "sidecar", Image: "sidecar-image"},
					},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.BeginFeature()
	// Replace
	m.EnsureContainer(corev1.Container{Name: "app", Image: "app-new-image"})
	// Remove
	m.RemoveContainer("sidecar")
	// Append
	m.EnsureContainer(corev1.Container{Name: "new-container", Image: newImage})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, deploy.Spec.Template.Spec.Containers, 2)
	assert.Equal(t, "app", deploy.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-new-image", deploy.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "new-container", deploy.Spec.Template.Spec.Containers[1].Name)
	assert.Equal(t, newImage, deploy.Spec.Template.Spec.Containers[1].Image)
}

func TestMutator_InitContainerPresence(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "init-1-image"},
					},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.BeginFeature()
	m.EnsureInitContainer(corev1.Container{Name: "init-2", Image: "init-2-image"})
	m.RemoveInitContainers([]string{"init-1"})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, deploy.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-2", deploy.Spec.Template.Spec.InitContainers[0].Name)
}

func TestMutator_SelectorSnapshotSemantics(t *testing.T) {
	const appV2 = "app-v2"
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app-image"},
					},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.BeginFeature()

	// First edit renames the container
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = appV2
		return nil
	})

	// Second edit should still match using "app" selector because of snapshot
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "app-image-updated"
		return nil
	})

	// Third edit targeting "app-v2" should NOT match in this apply pass
	m.EditContainers(selectors.ContainerNamed(appV2), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "should-not-be-set"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, appV2, deploy.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-image-updated", deploy.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_Ordering_PresenceBeforeEdit(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.BeginFeature()

	// Register edit first
	m.EditContainers(selectors.ContainerNamed("new-app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "edited-image"
		return nil
	})

	// Register presence later
	m.EnsureContainer(corev1.Container{Name: "new-app", Image: "original-image"})

	err := m.Apply()
	require.NoError(t, err)

	// It should work because presence happens before edits in Apply()
	require.Len(t, deploy.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "edited-image", deploy.Spec.Template.Spec.Containers[0].Image)
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
	m.BeginFeature()

	// These should all be no-ops and not panic
	m.EditContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditContainers(selectors.AllContainers(), nil)
	m.EditPodSpec(nil)
	m.EditPodTemplateMetadata(nil)
	m.EditObjectMetadata(nil)
	m.EditDeploymentSpec(nil)

	err := m.Apply()
	assert.NoError(t, err)
}

func TestMutator_CrossFeatureOrdering(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "v1"}},
				},
			},
		},
	}

	m := NewMutator(deploy)

	// Feature A: sets replicas to 2, image to v2
	m.BeginFeature()
	m.EnsureReplicas(2)
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2"
		return nil
	})

	// Feature B: sets replicas to 3, image to v3
	m.BeginFeature()
	m.EnsureReplicas(3)
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v3"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	// Feature B should win
	assert.Equal(t, int32(3), *deploy.Spec.Replicas)
	assert.Equal(t, "v3", deploy.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_WithinFeatureCategoryOrdering(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "original-name"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.BeginFeature()

	var executionOrder []string

	// We register them in reverse order of expected execution
	m.EditContainers(selectors.AllContainers(), func(_ *editors.ContainerEditor) error {
		executionOrder = append(executionOrder, "container")
		return nil
	})
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		executionOrder = append(executionOrder, "podspec")
		return nil
	})
	m.EditPodTemplateMetadata(func(_ *editors.ObjectMetaEditor) error {
		executionOrder = append(executionOrder, "podmeta")
		return nil
	})
	m.EditDeploymentSpec(func(_ *editors.DeploymentSpecEditor) error {
		executionOrder = append(executionOrder, "deploymentspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		executionOrder = append(executionOrder, "deploymentmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expectedOrder := []string{
		"deploymentmeta",
		"deploymentspec",
		"podmeta",
		"podspec",
		"container",
	}
	assert.Equal(t, expectedOrder, executionOrder)
}

func TestMutator_CrossFeatureVisibility(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(deploy)

	// Feature A renames container
	m.BeginFeature()
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = "app-v2"
		return nil
	})

	// Feature B selects by the new name - this should work!
	m.BeginFeature()
	m.EditContainers(selectors.ContainerNamed("app-v2"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2-image"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, "app-v2", deploy.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "v2-image", deploy.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_InitContainer_OrderingAndSnapshots(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(deploy)
	m.BeginFeature()

	// 1. Add init-1
	m.EnsureInitContainer(corev1.Container{Name: "init-1", Image: "v1"})

	// 2. Edit init-1 (it's present in the same feature's phase)
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v1-edited"
		return nil
	})

	// 3. Rename it inside the edit phase
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = "init-1-renamed"
		return nil
	})

	// 4. Selector targeting "init-1" should still match because of snapshot in same phase
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v1-final"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, deploy.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-1-renamed", deploy.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "v1-final", deploy.Spec.Template.Spec.InitContainers[0].Image)
}
