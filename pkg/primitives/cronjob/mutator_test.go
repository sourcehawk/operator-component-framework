package cronjob

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewMutator(t *testing.T) {
	cj := &batchv1.CronJob{}
	m := NewMutator(cj)
	assert.NotNil(t, m)
	assert.Equal(t, cj, m.current)
}

func TestMutator_EditObjectMetadata(t *testing.T) {
	cj := &batchv1.CronJob{}
	m := NewMutator(cj)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Labels = map[string]string{"cronjob": "label"}
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "label", cj.Labels["cronjob"])
}

func TestMutator_EditCronJobSpec(t *testing.T) {
	cj := &batchv1.CronJob{}
	m := NewMutator(cj)
	m.EditCronJobSpec(func(e *editors.CronJobSpecEditor) error {
		e.SetSchedule("*/5 * * * *")
		e.SetConcurrencyPolicy(batchv1.ForbidConcurrent)
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "*/5 * * * *", cj.Spec.Schedule)
	assert.Equal(t, batchv1.ForbidConcurrent, cj.Spec.ConcurrencyPolicy)
}

func TestMutator_EditJobSpec(t *testing.T) {
	cj := &batchv1.CronJob{}
	m := NewMutator(cj)
	m.EditJobSpec(func(e *editors.JobSpecEditor) error {
		e.SetBackoffLimit(3)
		e.SetCompletions(1)
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, int32(3), *cj.Spec.JobTemplate.Spec.BackoffLimit)
	assert.Equal(t, int32(1), *cj.Spec.JobTemplate.Spec.Completions)
}

func TestMutator_EditPodTemplateMetadata(t *testing.T) {
	cj := &batchv1.CronJob{}
	m := NewMutator(cj)
	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Annotations = map[string]string{"pod": "ann"}
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "ann", cj.Spec.JobTemplate.Spec.Template.Annotations["pod"])
}

func TestMutator_EditPodSpec(t *testing.T) {
	cj := &batchv1.CronJob{}
	m := NewMutator(cj)
	m.EditPodSpec(func(e *editors.PodSpecEditor) error {
		e.Raw().ServiceAccountName = "my-sa"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "my-sa", cj.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName)
}

func TestMutator_EditContainers(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "c1"},
								{Name: "c2"},
							},
						},
					},
				},
			},
		},
	}

	m := NewMutator(cj)
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

	assert.Equal(t, "c1-image", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[1].Image)
	assert.Equal(t, "GLOBAL", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "GLOBAL", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[1].Env[0].Name)
}

func TestMutator_EnvVars(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
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
			},
		},
	}

	m := NewMutator(cj)
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CHANGE", Value: "new"})
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "ADD", Value: "added"})
	m.RemoveContainerEnvVars([]string{"REMOVE", "NONEXISTENT"})

	err := m.Apply()
	require.NoError(t, err)

	env := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env
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
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
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
			},
		},
	}

	m := NewMutator(cj)
	m.EnsureContainerArg("--change=new")
	m.EnsureContainerArg("--add")
	m.RemoveContainerArgs([]string{"--remove", "--nonexistent"})

	err := m.Apply()
	require.NoError(t, err)

	args := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--keep")
	assert.Contains(t, args, "--change=old")
	assert.Contains(t, args, "--change=new")
	assert.Contains(t, args, "--add")
	assert.NotContains(t, args, "--remove")
}

func TestMutator_ContainerPresence(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "app", Image: "app-image"},
								{Name: "sidecar", Image: "sidecar-image"},
							},
						},
					},
				},
			},
		},
	}

	m := NewMutator(cj)
	m.EnsureContainer(corev1.Container{Name: "app", Image: "app-new-image"})
	m.RemoveContainer("sidecar")
	m.EnsureContainer(corev1.Container{Name: "new-container", Image: "new-image"})

	err := m.Apply()
	require.NoError(t, err)

	containers := cj.Spec.JobTemplate.Spec.Template.Spec.Containers
	require.Len(t, containers, 2)
	assert.Equal(t, "app", containers[0].Name)
	assert.Equal(t, "app-new-image", containers[0].Image)
	assert.Equal(t, "new-container", containers[1].Name)
	assert.Equal(t, "new-image", containers[1].Image)
}

func TestMutator_InitContainers(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{Name: "init-1", Image: "old-image"},
							},
						},
					},
				},
			},
		},
	}

	m := NewMutator(cj)
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "new-image"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "new-image", cj.Spec.JobTemplate.Spec.Template.Spec.InitContainers[0].Image)
}

func TestMutator_InitContainerPresence(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{Name: "init-1", Image: "init-1-image"},
							},
						},
					},
				},
			},
		},
	}

	m := NewMutator(cj)
	m.EnsureInitContainer(corev1.Container{Name: "init-2", Image: "init-2-image"})
	m.RemoveInitContainers([]string{"init-1"})

	err := m.Apply()
	require.NoError(t, err)

	initContainers := cj.Spec.JobTemplate.Spec.Template.Spec.InitContainers
	require.Len(t, initContainers, 1)
	assert.Equal(t, "init-2", initContainers[0].Name)
}

func TestMutator_Errors(t *testing.T) {
	cj := &batchv1.CronJob{}
	m := NewMutator(cj)
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		return errors.New("boom")
	})

	err := m.Apply()
	assert.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestMutator_NilSafety(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "main"}},
						},
					},
				},
			},
		},
	}
	m := NewMutator(cj)

	m.EditContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditContainers(selectors.AllContainers(), nil)
	m.EditInitContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditInitContainers(selectors.AllContainers(), nil)
	m.EditPodSpec(nil)
	m.EditPodTemplateMetadata(nil)
	m.EditObjectMetadata(nil)
	m.EditCronJobSpec(nil)
	m.EditJobSpec(nil)

	err := m.Apply()
	assert.NoError(t, err)
}

func TestMutator_Order(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "main"}},
						},
					},
				},
			},
		},
	}

	var order []string

	m := NewMutator(cj)
	// Register in reverse order to verify execution order
	m.EditContainers(selectors.AllContainers(), func(_ *editors.ContainerEditor) error {
		order = append(order, "container")
		return nil
	})
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		order = append(order, "podspec")
		return nil
	})
	m.EditPodTemplateMetadata(func(_ *editors.ObjectMetaEditor) error {
		order = append(order, "podmeta")
		return nil
	})
	m.EditJobSpec(func(_ *editors.JobSpecEditor) error {
		order = append(order, "jobspec")
		return nil
	})
	m.EditCronJobSpec(func(_ *editors.CronJobSpecEditor) error {
		order = append(order, "cronjobspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		order = append(order, "cronjobmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expected := []string{"cronjobmeta", "cronjobspec", "jobspec", "podmeta", "podspec", "container"}
	assert.Equal(t, expected, order)
}

func TestMutator_CrossFeatureOrdering(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "app", Image: "v1"}},
						},
					},
				},
			},
		},
	}

	m := NewMutator(cj)

	// Feature A
	m.BeginFeature()
	m.EditCronJobSpec(func(e *editors.CronJobSpecEditor) error {
		e.SetSchedule("*/10 * * * *")
		return nil
	})
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2"
		return nil
	})

	// Feature B
	m.BeginFeature()
	m.EditCronJobSpec(func(e *editors.CronJobSpecEditor) error {
		e.SetSchedule("0 * * * *")
		return nil
	})
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v3"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, "0 * * * *", cj.Spec.Schedule)
	assert.Equal(t, "v3", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_SelectorSnapshotSemantics(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "app", Image: "app-image"},
							},
						},
					},
				},
			},
		},
	}

	m := NewMutator(cj)

	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = "app-v2"
		return nil
	})

	// Should still match using snapshot
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "app-image-updated"
		return nil
	})

	// Should NOT match in this apply pass
	m.EditContainers(selectors.ContainerNamed("app-v2"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "should-not-be-set"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, "app-v2", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-image-updated", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_Ordering_PresenceBeforeEdit(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{},
						},
					},
				},
			},
		},
	}

	m := NewMutator(cj)

	// Register edit first
	m.EditContainers(selectors.ContainerNamed("new-app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "edited-image"
		return nil
	})

	// Register presence later
	m.EnsureContainer(corev1.Container{Name: "new-app", Image: "original-image"})

	err := m.Apply()
	require.NoError(t, err)

	containers := cj.Spec.JobTemplate.Spec.Template.Spec.Containers
	require.Len(t, containers, 1)
	assert.Equal(t, "edited-image", containers[0].Image)
}

func TestMutator_CrossFeatureVisibility(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "app"}},
						},
					},
				},
			},
		},
	}

	m := NewMutator(cj)

	// Feature A renames container
	m.BeginFeature()
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = "app-v2"
		return nil
	})

	// Feature B selects by the new name
	m.BeginFeature()
	m.EditContainers(selectors.ContainerNamed("app-v2"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2-image"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, "app-v2", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "v2-image", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_EditMetadata(t *testing.T) {
	cj := &batchv1.CronJob{}
	m := NewMutator(cj)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Labels = map[string]string{"cronjob": "label"}
		return nil
	})
	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Annotations = map[string]string{"pod": "ann"}
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "label", cj.Labels["cronjob"])
	assert.Equal(t, "ann", cj.Spec.JobTemplate.Spec.Template.Annotations["pod"])
}

func TestMutator_WithinFeatureCategoryOrdering(t *testing.T) {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "original-name"},
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "app"}},
						},
					},
				},
			},
		},
	}

	m := NewMutator(cj)

	var executionOrder []string

	// Register in reverse order of expected execution
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
	m.EditJobSpec(func(_ *editors.JobSpecEditor) error {
		executionOrder = append(executionOrder, "jobspec")
		return nil
	})
	m.EditCronJobSpec(func(_ *editors.CronJobSpecEditor) error {
		executionOrder = append(executionOrder, "cronjobspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		executionOrder = append(executionOrder, "cronjobmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expectedOrder := []string{
		"cronjobmeta",
		"cronjobspec",
		"jobspec",
		"podmeta",
		"podspec",
		"container",
	}
	assert.Equal(t, expectedOrder, executionOrder)
}
