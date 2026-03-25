package job

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
	job := &batchv1.Job{}
	m := NewMutator(job)
	assert.NotNil(t, m)
	assert.Equal(t, job, m.current)
	require.Len(t, m.plans, 1, "NewMutator must create exactly one plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the initial plan")
}

func TestNextFeature_AddsOnePlan(t *testing.T) {
	job := &batchv1.Job{}
	m := NewMutator(job)

	require.Len(t, m.plans, 1, "constructor must create the initial plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the initial plan")

	m.NextFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestNextFeature_IsolatesFeaturePlans(t *testing.T) {
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}
	m := NewMutator(job)

	// Record mutations in the first feature plan (auto-created by constructor)
	m.EditJobSpec(func(e *editors.JobSpecEditor) error {
		e.SetBackoffLimit(3)
		return nil
	})
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v1"
		return nil
	})

	// Start a new feature and record different mutations
	m.NextFeature()
	m.EditJobSpec(func(e *editors.JobSpecEditor) error {
		e.SetBackoffLimit(5)
		return nil
	})

	// First plan should have its edits, second plan should have its own
	assert.Len(t, m.plans[0].jobSpecEdits, 1, "first plan should have one spec edit")
	assert.Len(t, m.plans[0].containerEdits, 1, "first plan should have one container edit")
	assert.Len(t, m.plans[1].jobSpecEdits, 1, "second plan should have one spec edit")
	assert.Empty(t, m.plans[1].containerEdits, "second plan should have no container edits")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	job := &batchv1.Job{}
	m := NewMutator(job)
	m.EditJobSpec(func(e *editors.JobSpecEditor) error {
		e.SetBackoffLimit(3)
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Len(t, m.plans, 1, "no extra plans should be created during Apply")
	assert.Equal(t, int32(3), *job.Spec.BackoffLimit)
}

func TestMutator_EnvVars(t *testing.T) {
	job := &batchv1.Job{
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
	}

	m := NewMutator(job)
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CHANGE", Value: "new"})
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "ADD", Value: "added"})
	m.RemoveContainerEnvVar("REMOVE")

	err := m.Apply()
	require.NoError(t, err)

	env := job.Spec.Template.Spec.Containers[0].Env
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

func TestMutator_EditContainers(t *testing.T) {
	job := &batchv1.Job{
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
	}

	m := NewMutator(job)
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

	assert.Equal(t, "c1-image", job.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "", job.Spec.Template.Spec.Containers[1].Image)
	assert.Equal(t, "GLOBAL", job.Spec.Template.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "GLOBAL", job.Spec.Template.Spec.Containers[1].Env[0].Name)
}

func TestMutator_EditPodSpec(t *testing.T) {
	job := &batchv1.Job{}
	m := NewMutator(job)
	m.EditPodSpec(func(e *editors.PodSpecEditor) error {
		e.Raw().ServiceAccountName = "my-sa"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "my-sa", job.Spec.Template.Spec.ServiceAccountName)
}

func TestMutator_EditJobSpec(t *testing.T) {
	job := &batchv1.Job{}
	m := NewMutator(job)
	m.EditJobSpec(func(e *editors.JobSpecEditor) error {
		e.SetBackoffLimit(5)
		e.SetCompletions(3)
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	require.NotNil(t, job.Spec.BackoffLimit)
	assert.Equal(t, int32(5), *job.Spec.BackoffLimit)
	require.NotNil(t, job.Spec.Completions)
	assert.Equal(t, int32(3), *job.Spec.Completions)
}

func TestMutator_EditMetadata(t *testing.T) {
	job := &batchv1.Job{}
	m := NewMutator(job)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Labels = map[string]string{"job": "label"}
		return nil
	})
	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Annotations = map[string]string{"pod": "ann"}
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "label", job.Labels["job"])
	assert.Equal(t, "ann", job.Spec.Template.Annotations["pod"])
}

func TestMutator_Errors(t *testing.T) {
	job := &batchv1.Job{}
	m := NewMutator(job)
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		return errors.New("boom")
	})

	err := m.Apply()
	assert.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestMutator_Order(t *testing.T) {
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
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

	m := NewMutator(job)
	// Register in reverse order to verify fixed category ordering
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
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		order = append(order, "jobmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expected := []string{"jobmeta", "jobspec", "podmeta", "podspec", "container"}
	assert.Equal(t, expected, order)
}

func TestMutator_ContainerPresence(t *testing.T) {
	job := &batchv1.Job{
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
	}

	m := NewMutator(job)
	m.EnsureContainer(corev1.Container{Name: "app", Image: "app-new-image"})
	m.RemoveContainer("sidecar")
	m.EnsureContainer(corev1.Container{Name: "new-container", Image: "new-image"})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, job.Spec.Template.Spec.Containers, 2)
	assert.Equal(t, "app", job.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-new-image", job.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "new-container", job.Spec.Template.Spec.Containers[1].Name)
	assert.Equal(t, "new-image", job.Spec.Template.Spec.Containers[1].Image)
}

func TestMutator_InitContainerPresence(t *testing.T) {
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "init-1-image"},
					},
				},
			},
		},
	}

	m := NewMutator(job)
	m.EnsureInitContainer(corev1.Container{Name: "init-2", Image: "init-2-image"})
	m.RemoveInitContainers([]string{"init-1"})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, job.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-2", job.Spec.Template.Spec.InitContainers[0].Name)
}

func TestMutator_SelectorSnapshotSemantics(t *testing.T) {
	const appV2 = "app-v2"
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app-image"},
					},
				},
			},
		},
	}

	m := NewMutator(job)

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

	assert.Equal(t, appV2, job.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-image-updated", job.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_NilSafety(t *testing.T) {
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
	m := NewMutator(job)

	// These should all be no-ops and not panic
	m.EditContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditContainers(selectors.AllContainers(), nil)
	m.EditInitContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditInitContainers(selectors.AllContainers(), nil)
	m.EditPodSpec(nil)
	m.EditPodTemplateMetadata(nil)
	m.EditObjectMetadata(nil)
	m.EditJobSpec(nil)

	err := m.Apply()
	assert.NoError(t, err)
}

func TestMutator_CrossFeatureOrdering(t *testing.T) {
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "v1"}},
				},
			},
		},
	}

	m := NewMutator(job)

	// Feature A: sets backoff to 2, image to v2
	m.EditJobSpec(func(e *editors.JobSpecEditor) error {
		e.SetBackoffLimit(2)
		return nil
	})
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2"
		return nil
	})

	// Feature B: sets backoff to 3, image to v3
	m.NextFeature()
	m.EditJobSpec(func(e *editors.JobSpecEditor) error {
		e.SetBackoffLimit(3)
		return nil
	})
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v3"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	// Feature B should win
	assert.Equal(t, int32(3), *job.Spec.BackoffLimit)
	assert.Equal(t, "v3", job.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_WithinFeatureCategoryOrdering(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "original-name"},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(job)

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
	m.EditJobSpec(func(_ *editors.JobSpecEditor) error {
		executionOrder = append(executionOrder, "jobspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		executionOrder = append(executionOrder, "jobmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expectedOrder := []string{
		"jobmeta",
		"jobspec",
		"podmeta",
		"podspec",
		"container",
	}
	assert.Equal(t, expectedOrder, executionOrder)
}

func TestMutator_CrossFeatureVisibility(t *testing.T) {
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(job)

	// Feature A renames container
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = "app-v2"
		return nil
	})

	// Feature B selects by the new name
	m.NextFeature()
	m.EditContainers(selectors.ContainerNamed("app-v2"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2-image"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, "app-v2", job.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "v2-image", job.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_PresenceBeforeEdit(t *testing.T) {
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(job)

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
	require.Len(t, job.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "edited-image", job.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_InitContainers(t *testing.T) {
	const newImage = "new-image"
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "old-image"},
					},
				},
			},
		},
	}

	m := NewMutator(job)
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = newImage
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, newImage, job.Spec.Template.Spec.InitContainers[0].Image)
}

func TestMutator_InitContainer_OrderingAndSnapshots(t *testing.T) {
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(job)

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

	require.Len(t, job.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-1-renamed", job.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "v1-final", job.Spec.Template.Spec.InitContainers[0].Image)
}
