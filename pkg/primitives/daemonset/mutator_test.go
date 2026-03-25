package daemonset

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
)

func TestMutator_EnvVars(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
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

	m := NewMutator(ds)
	m.BeginFeature()
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CHANGE", Value: "new"})
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "ADD", Value: "added"})
	m.RemoveContainerEnvVars([]string{"REMOVE", "NONEXISTENT"})

	err := m.Apply()
	require.NoError(t, err)

	env := ds.Spec.Template.Spec.Containers[0].Env
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
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
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

	m := NewMutator(ds)
	m.BeginFeature()
	m.EnsureContainerArg("--change=new")
	m.EnsureContainerArg("--add")
	m.RemoveContainerArgs([]string{"--remove", "--nonexistent"})

	err := m.Apply()
	require.NoError(t, err)

	args := ds.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--keep")
	assert.Contains(t, args, "--change=old")
	assert.Contains(t, args, "--change=new")
	assert.Contains(t, args, "--add")
	assert.NotContains(t, args, "--remove")
}

func TestNewMutator(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	m := NewMutator(ds)
	assert.NotNil(t, m)
	assert.Equal(t, ds, m.current)
	assert.Empty(t, m.plans, "NewMutator must not create any plans")
	assert.Nil(t, m.active, "active plan must not be set")
}

func TestBeginFeature_AddsExactlyOnePlan(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	m := NewMutator(ds)

	m.BeginFeature()
	require.Len(t, m.plans, 1, "BeginFeature must add exactly one plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the new plan")

	m.BeginFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestBeginFeature_IsolatesFeaturePlans(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}
	m := NewMutator(ds)

	// Record mutations in the first feature plan
	m.BeginFeature()
	m.EditDaemonSetSpec(func(e *editors.DaemonSetSpecEditor) error {
		e.SetMinReadySeconds(10)
		return nil
	})
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v1"
		return nil
	})

	// Start a new feature and record different mutations
	m.BeginFeature()
	m.EditDaemonSetSpec(func(e *editors.DaemonSetSpecEditor) error {
		e.SetMinReadySeconds(20)
		return nil
	})

	// First plan should have its edits, second plan should have its own
	assert.Len(t, m.plans[0].daemonsetSpecEdits, 1, "first plan should have one spec edit")
	assert.Len(t, m.plans[0].containerEdits, 1, "first plan should have one container edit")
	assert.Len(t, m.plans[1].daemonsetSpecEdits, 1, "second plan should have one spec edit")
	assert.Empty(t, m.plans[1].containerEdits, "second plan should have no container edits")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	m := NewMutator(ds)
	m.BeginFeature()
	m.EditDaemonSetSpec(func(e *editors.DaemonSetSpecEditor) error {
		e.SetMinReadySeconds(10)
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Len(t, m.plans, 1, "no extra plans should be created during Apply")
	assert.Equal(t, int32(10), ds.Spec.MinReadySeconds)
}

func TestMutator_EditContainers(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
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

	m := NewMutator(ds)
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

	assert.Equal(t, "c1-image", ds.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "", ds.Spec.Template.Spec.Containers[1].Image)
	assert.Equal(t, "GLOBAL", ds.Spec.Template.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "GLOBAL", ds.Spec.Template.Spec.Containers[1].Env[0].Name)
}

func TestMutator_EditPodSpec(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	m := NewMutator(ds)
	m.BeginFeature()
	m.EditPodSpec(func(e *editors.PodSpecEditor) error {
		e.Raw().ServiceAccountName = "my-sa"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "my-sa", ds.Spec.Template.Spec.ServiceAccountName)
}

func TestMutator_EditDaemonSetSpec(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	m := NewMutator(ds)
	m.BeginFeature()
	m.EditDaemonSetSpec(func(e *editors.DaemonSetSpecEditor) error {
		e.SetMinReadySeconds(10)
		e.SetRevisionHistoryLimit(5)
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, int32(10), ds.Spec.MinReadySeconds)
	assert.Equal(t, int32(5), *ds.Spec.RevisionHistoryLimit)
}

func TestMutator_EditMetadata(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	m := NewMutator(ds)
	m.BeginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Labels = map[string]string{"ds": "label"}
		return nil
	})
	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Annotations = map[string]string{"pod": "ann"}
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "label", ds.Labels["ds"])
	assert.Equal(t, "ann", ds.Spec.Template.Annotations["pod"])
}

func TestMutator_Errors(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	m := NewMutator(ds)
	m.BeginFeature()
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		return errors.New("boom")
	})

	err := m.Apply()
	assert.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestMutator_Order(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
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

	m := NewMutator(ds)
	m.BeginFeature()
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
	m.EditDaemonSetSpec(func(_ *editors.DaemonSetSpecEditor) error {
		order = append(order, "dsspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		order = append(order, "dsmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expected := []string{"dsmeta", "dsspec", "podmeta", "podspec", "container"}
	assert.Equal(t, expected, order)
}

func TestMutator_InitContainers(t *testing.T) {
	const newImage = "new-image"
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "old-image"},
					},
				},
			},
		},
	}

	m := NewMutator(ds)
	m.BeginFeature()
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = newImage
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, newImage, ds.Spec.Template.Spec.InitContainers[0].Image)
}

func TestMutator_ContainerPresence(t *testing.T) {
	const newImage = "new-image"
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
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

	m := NewMutator(ds)
	m.BeginFeature()
	m.EnsureContainer(corev1.Container{Name: "app", Image: "app-new-image"})
	m.RemoveContainer("sidecar")
	m.EnsureContainer(corev1.Container{Name: "new-container", Image: newImage})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, ds.Spec.Template.Spec.Containers, 2)
	assert.Equal(t, "app", ds.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-new-image", ds.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "new-container", ds.Spec.Template.Spec.Containers[1].Name)
	assert.Equal(t, newImage, ds.Spec.Template.Spec.Containers[1].Image)
}

func TestMutator_InitContainerPresence(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "init-1-image"},
					},
				},
			},
		},
	}

	m := NewMutator(ds)
	m.BeginFeature()
	m.EnsureInitContainer(corev1.Container{Name: "init-2", Image: "init-2-image"})
	m.RemoveInitContainers([]string{"init-1"})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, ds.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-2", ds.Spec.Template.Spec.InitContainers[0].Name)
}

func TestMutator_SelectorSnapshotSemantics(t *testing.T) {
	const appV2 = "app-v2"
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app-image"},
					},
				},
			},
		},
	}

	m := NewMutator(ds)
	m.BeginFeature()

	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = appV2
		return nil
	})

	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "app-image-updated"
		return nil
	})

	m.EditContainers(selectors.ContainerNamed(appV2), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "should-not-be-set"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, appV2, ds.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-image-updated", ds.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_Ordering_PresenceBeforeEdit(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(ds)
	m.BeginFeature()

	m.EditContainers(selectors.ContainerNamed("new-app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "edited-image"
		return nil
	})

	m.EnsureContainer(corev1.Container{Name: "new-app", Image: "original-image"})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, ds.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "edited-image", ds.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_NilSafety(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
	m := NewMutator(ds)
	m.BeginFeature()

	m.EditContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditContainers(selectors.AllContainers(), nil)
	m.EditPodSpec(nil)
	m.EditPodTemplateMetadata(nil)
	m.EditObjectMetadata(nil)
	m.EditDaemonSetSpec(nil)

	err := m.Apply()
	assert.NoError(t, err)
}

func TestMutator_CrossFeatureOrdering(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "v1"}},
				},
			},
		},
	}

	m := NewMutator(ds)

	// Feature A: sets min ready seconds, image to v2
	m.BeginFeature()
	m.EditDaemonSetSpec(func(e *editors.DaemonSetSpecEditor) error {
		e.SetMinReadySeconds(10)
		return nil
	})
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2"
		return nil
	})

	// Feature B: sets min ready seconds, image to v3
	m.BeginFeature()
	m.EditDaemonSetSpec(func(e *editors.DaemonSetSpecEditor) error {
		e.SetMinReadySeconds(20)
		return nil
	})
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v3"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, int32(20), ds.Spec.MinReadySeconds)
	assert.Equal(t, "v3", ds.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_WithinFeatureCategoryOrdering(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "original-name"},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(ds)
	m.BeginFeature()

	var executionOrder []string

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
	m.EditDaemonSetSpec(func(_ *editors.DaemonSetSpecEditor) error {
		executionOrder = append(executionOrder, "daemonsetspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		executionOrder = append(executionOrder, "daemonsetmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expectedOrder := []string{
		"daemonsetmeta",
		"daemonsetspec",
		"podmeta",
		"podspec",
		"container",
	}
	assert.Equal(t, expectedOrder, executionOrder)
}

func TestMutator_CrossFeatureVisibility(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(ds)

	m.BeginFeature()
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = "app-v2"
		return nil
	})

	m.BeginFeature()
	m.EditContainers(selectors.ContainerNamed("app-v2"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2-image"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, "app-v2", ds.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "v2-image", ds.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_InitContainer_OrderingAndSnapshots(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(ds)
	m.BeginFeature()

	m.EnsureInitContainer(corev1.Container{Name: "init-1", Image: "v1"})

	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v1-edited"
		return nil
	})

	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = "init-1-renamed"
		return nil
	})

	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v1-final"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, ds.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-1-renamed", ds.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "v1-final", ds.Spec.Template.Spec.InitContainers[0].Image)
}

func TestMutator_PanicsWithoutBeginFeature(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "nginx"}},
				},
			},
		},
	}
	m := NewMutator(ds)

	assert.PanicsWithValue(t, "daemonset.Mutator: BeginFeature() must be called before registering mutations", func() {
		m.EnsureContainer(corev1.Container{Name: "new"})
	})
	assert.PanicsWithValue(t, "daemonset.Mutator: BeginFeature() must be called before registering mutations", func() {
		m.EditObjectMetadata(func(*editors.ObjectMetaEditor) error { return nil })
	})
}
