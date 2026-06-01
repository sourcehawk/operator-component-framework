package statefulset

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestMutator_EnvVars(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
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

	m := NewMutator(sts)
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CHANGE", Value: "new"})
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "ADD", Value: "added"})
	m.RemoveContainerEnvVars([]string{"REMOVE", "NONEXISTENT"})

	err := m.Apply()
	require.NoError(t, err)

	env := sts.Spec.Template.Spec.Containers[0].Env
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
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
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

	m := NewMutator(sts)
	m.EnsureContainerArg("--change=new")
	m.EnsureContainerArg("--add")
	m.RemoveContainerArgs([]string{"--remove", "--nonexistent"})

	err := m.Apply()
	require.NoError(t, err)

	args := sts.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--keep")
	assert.Contains(t, args, "--change=old")
	assert.Contains(t, args, "--change=new")
	assert.Contains(t, args, "--add")
	assert.NotContains(t, args, "--remove")
}

func TestMutator_Replicas(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(int32(3)),
		},
	}

	m := NewMutator(sts)
	m.EnsureReplicas(5)

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, int32(5), *sts.Spec.Replicas)
}

func TestNewMutator(t *testing.T) {
	sts := &appsv1.StatefulSet{}
	m := NewMutator(sts)
	assert.NotNil(t, m)
	assert.Equal(t, sts, m.current)
	require.Len(t, m.plans, 1, "NewMutator must create the initial feature plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the initial plan")
}

func TestNextFeature_AddsExactlyOnePlan(t *testing.T) {
	sts := &appsv1.StatefulSet{}
	m := NewMutator(sts)

	require.Len(t, m.plans, 1, "NewMutator must create the initial plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the initial plan")

	m.NextFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestNextFeature_IsolatesFeaturePlans(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}
	m := NewMutator(sts)

	// Record mutations in the initial feature plan
	m.EnsureReplicas(3)
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v1"
		return nil
	})

	// Start a new feature and record a different mutation
	m.NextFeature()
	m.EnsureReplicas(5)

	// First plan should have its edits, second plan should have its own
	assert.Len(t, m.plans[0].statefulsetSpecEdits, 1, "first plan should have one spec edit")
	assert.Len(t, m.plans[0].containerEdits, 1, "first plan should have one container edit")
	assert.Len(t, m.plans[1].statefulsetSpecEdits, 1, "second plan should have one spec edit")
	assert.Empty(t, m.plans[1].containerEdits, "second plan should have no container edits")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	sts := &appsv1.StatefulSet{}
	m := NewMutator(sts)
	m.EnsureReplicas(3)

	require.NoError(t, m.Apply())
	assert.Len(t, m.plans, 1)
}

func TestMutator_EditContainers(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
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

	m := NewMutator(sts)
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

	assert.Equal(t, "c1-image", sts.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "", sts.Spec.Template.Spec.Containers[1].Image)
	assert.Equal(t, "GLOBAL", sts.Spec.Template.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "GLOBAL", sts.Spec.Template.Spec.Containers[1].Env[0].Name)
}

func TestMutator_EditPodSpec(t *testing.T) {
	sts := &appsv1.StatefulSet{}
	m := NewMutator(sts)
	m.EditPodSpec(func(e *editors.PodSpecEditor) error {
		e.Raw().ServiceAccountName = "my-sa"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "my-sa", sts.Spec.Template.Spec.ServiceAccountName)
}

func TestMutator_EditStatefulSetSpec(t *testing.T) {
	sts := &appsv1.StatefulSet{}
	m := NewMutator(sts)
	m.EditStatefulSetSpec(func(e *editors.StatefulSetSpecEditor) error {
		e.SetServiceName("my-service")
		e.SetMinReadySeconds(10)
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "my-service", sts.Spec.ServiceName)
	assert.Equal(t, int32(10), sts.Spec.MinReadySeconds)
}

func TestMutator_EditMetadata(t *testing.T) {
	sts := &appsv1.StatefulSet{}
	m := NewMutator(sts)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Labels = map[string]string{"sts": "label"}
		return nil
	})
	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Annotations = map[string]string{"pod": "ann"}
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "label", sts.Labels["sts"])
	assert.Equal(t, "ann", sts.Spec.Template.Annotations["pod"])
}

func TestMutator_Errors(t *testing.T) {
	sts := &appsv1.StatefulSet{}
	m := NewMutator(sts)
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		return errors.New("boom")
	})

	err := m.Apply()
	assert.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestMutator_Order(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
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

	m := NewMutator(sts)
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
	m.EditStatefulSetSpec(func(_ *editors.StatefulSetSpecEditor) error {
		order = append(order, "stsspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		order = append(order, "stsmeta")
		return nil
	})
	m.EnsureReplicas(3)

	err := m.Apply()
	require.NoError(t, err)

	expected := []string{"stsmeta", "stsspec", "podmeta", "podspec", "container"}
	assert.Equal(t, expected, order)
	assert.Equal(t, int32(3), *sts.Spec.Replicas)
}

func TestMutator_InitContainers(t *testing.T) {
	const newImage = "new-image"
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "old-image"},
					},
				},
			},
		},
	}

	m := NewMutator(sts)
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = newImage
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, newImage, sts.Spec.Template.Spec.InitContainers[0].Image)
}

func TestMutator_ContainerPresence(t *testing.T) {
	const newImage = "new-image"
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
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

	m := NewMutator(sts)
	m.EnsureContainer(corev1.Container{Name: "app", Image: "app-new-image"})
	m.RemoveContainer("sidecar")
	m.EnsureContainer(corev1.Container{Name: "new-container", Image: newImage})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, sts.Spec.Template.Spec.Containers, 2)
	assert.Equal(t, "app", sts.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-new-image", sts.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "new-container", sts.Spec.Template.Spec.Containers[1].Name)
	assert.Equal(t, newImage, sts.Spec.Template.Spec.Containers[1].Image)
}

func TestMutator_InitContainerPresence(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "init-1-image"},
					},
				},
			},
		},
	}

	m := NewMutator(sts)
	m.EnsureInitContainer(corev1.Container{Name: "init-2", Image: "init-2-image"})
	m.RemoveInitContainers([]string{"init-1"})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, sts.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-2", sts.Spec.Template.Spec.InitContainers[0].Name)
}

func TestMutator_SelectorSnapshotSemantics(t *testing.T) {
	const appV2 = "app-v2"
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app-image"},
					},
				},
			},
		},
	}

	m := NewMutator(sts)
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

	assert.Equal(t, appV2, sts.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-image-updated", sts.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_Ordering_PresenceBeforeEdit(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(sts)

	m.EditContainers(selectors.ContainerNamed("new-app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "edited-image"
		return nil
	})

	m.EnsureContainer(corev1.Container{Name: "new-app", Image: "original-image"})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, sts.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "edited-image", sts.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_NilSafety(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
	m := NewMutator(sts)

	m.EditContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditContainers(selectors.AllContainers(), nil)
	m.EditInitContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditInitContainers(selectors.AllContainers(), nil)
	m.EditPodSpec(nil)
	m.EditPodTemplateMetadata(nil)
	m.EditObjectMetadata(nil)
	m.EditStatefulSetSpec(nil)

	err := m.Apply()
	assert.NoError(t, err)
}

func TestMutator_CrossFeatureOrdering(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To[int32](1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "v1"}},
				},
			},
		},
	}

	m := NewMutator(sts)

	m.EnsureReplicas(2)
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2"
		return nil
	})

	m.NextFeature()
	m.EnsureReplicas(3)
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v3"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, int32(3), *sts.Spec.Replicas)
	assert.Equal(t, "v3", sts.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_WithinFeatureCategoryOrdering(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "original-name"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(sts)

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
	m.EditStatefulSetSpec(func(_ *editors.StatefulSetSpecEditor) error {
		executionOrder = append(executionOrder, "statefulsetspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		executionOrder = append(executionOrder, "statefulsetmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expectedOrder := []string{
		"statefulsetmeta",
		"statefulsetspec",
		"podmeta",
		"podspec",
		"container",
	}
	assert.Equal(t, expectedOrder, executionOrder)
}

func TestMutator_CrossFeatureVisibility(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(sts)

	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = "app-v2"
		return nil
	})

	m.NextFeature()
	m.EditContainers(selectors.ContainerNamed("app-v2"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2-image"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, "app-v2", sts.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "v2-image", sts.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_InitContainer_OrderingAndSnapshots(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(sts)

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

	require.Len(t, sts.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-1-renamed", sts.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "v1-final", sts.Spec.Template.Spec.InitContainers[0].Image)
}

func TestMutator_VolumeClaimTemplates(t *testing.T) {
	t.Run("ensure adds new VCT", func(t *testing.T) {
		sts := &appsv1.StatefulSet{}
		m := NewMutator(sts)

		m.EnsureVolumeClaimTemplate(corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data"},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		})

		err := m.Apply()
		require.NoError(t, err)
		require.Len(t, sts.Spec.VolumeClaimTemplates, 1)
		assert.Equal(t, "data", sts.Spec.VolumeClaimTemplates[0].Name)
	})

	t.Run("ensure replaces existing VCT", func(t *testing.T) {
		sts := &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{ObjectMeta: metav1.ObjectMeta{Name: "data"}},
				},
			},
		}
		m := NewMutator(sts)

		m.EnsureVolumeClaimTemplate(corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data"},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("20Gi"),
					},
				},
			},
		})

		err := m.Apply()
		require.NoError(t, err)
		require.Len(t, sts.Spec.VolumeClaimTemplates, 1)
		qty := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
		assert.Equal(t, "20Gi", qty.String())
	})

	t.Run("remove VCT", func(t *testing.T) {
		sts := &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{ObjectMeta: metav1.ObjectMeta{Name: "data"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "logs"}},
				},
			},
		}
		m := NewMutator(sts)

		m.RemoveVolumeClaimTemplate("data")

		err := m.Apply()
		require.NoError(t, err)
		require.Len(t, sts.Spec.VolumeClaimTemplates, 1)
		assert.Equal(t, "logs", sts.Spec.VolumeClaimTemplates[0].Name)
	})

	t.Run("remove nonexistent VCT is no-op", func(t *testing.T) {
		sts := &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{ObjectMeta: metav1.ObjectMeta{Name: "data"}},
				},
			},
		}
		m := NewMutator(sts)

		m.RemoveVolumeClaimTemplate("nonexistent")

		err := m.Apply()
		require.NoError(t, err)
		require.Len(t, sts.Spec.VolumeClaimTemplates, 1)
	})

	t.Run("VCT ops are skipped on existing StatefulSet", func(t *testing.T) {
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "12345",
			},
			Spec: appsv1.StatefulSetSpec{
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{ObjectMeta: metav1.ObjectMeta{Name: "data"}},
				},
			},
		}
		m := NewMutator(sts)

		m.EnsureVolumeClaimTemplate(corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "new-volume"},
		})
		m.RemoveVolumeClaimTemplate("data")

		err := m.Apply()
		require.NoError(t, err)

		// VCT ops should be skipped since the StatefulSet already exists
		require.Len(t, sts.Spec.VolumeClaimTemplates, 1)
		assert.Equal(t, "data", sts.Spec.VolumeClaimTemplates[0].Name)
	})

	t.Run("VCT ops run after container edits", func(t *testing.T) {
		sts := &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app"}},
					},
				},
			},
		}

		var order []string
		m := NewMutator(sts)

		m.EnsureVolumeClaimTemplate(corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data"},
		})

		m.EditContainers(selectors.AllContainers(), func(_ *editors.ContainerEditor) error {
			order = append(order, "container")
			return nil
		})

		err := m.Apply()
		require.NoError(t, err)

		// Container edits run first (step 6), then VCT ops (step 9)
		assert.Equal(t, []string{"container"}, order)
		require.Len(t, sts.Spec.VolumeClaimTemplates, 1)
	})
}

// stubGate is a test-only feature.Gate that returns a fixed boolean.
type stubGate bool

func (g stubGate) Enabled() (bool, error) { return bool(g), nil }

func newSingleContainerSTS() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
}

func TestLiftMutation_CarriesAndInvokes(t *testing.T) {
	called := false
	agnostic := feature.Mutation[primitives.WorkloadMutator]{
		Name: "emit-env",
		Mutate: func(m primitives.WorkloadMutator) error {
			called = true
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}

	lifted := LiftMutation(agnostic)
	assert.Equal(t, "emit-env", lifted.Name)
	assert.Nil(t, lifted.Feature)
	require.NotNil(t, lifted.Mutate)

	sts := newSingleContainerSTS()
	m := NewMutator(sts)
	require.NoError(t, lifted.Mutate(m))
	require.NoError(t, m.Apply())

	assert.True(t, called)
	require.Len(t, sts.Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, "SHARED", sts.Spec.Template.Spec.Containers[0].Env[0].Name)
}

func TestLiftMutation_GateEnabledApplies(t *testing.T) {
	agnostic := feature.Mutation[primitives.WorkloadMutator]{
		Name:    "gated",
		Feature: stubGate(true),
		Mutate: func(m primitives.WorkloadMutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}

	lifted := LiftMutation(agnostic)
	conv := feature.Mutation[*Mutator](lifted)

	sts := newSingleContainerSTS()
	m := NewMutator(sts)
	require.NoError(t, conv.ApplyIntent(m))
	require.NoError(t, m.Apply())

	require.Len(t, sts.Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, "SHARED", sts.Spec.Template.Spec.Containers[0].Env[0].Name)
}

func TestLiftMutation_GateDisabledIsNoOp(t *testing.T) {
	agnostic := feature.Mutation[primitives.WorkloadMutator]{
		Name:    "gated",
		Feature: stubGate(false),
		Mutate: func(m primitives.WorkloadMutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}

	lifted := LiftMutation(agnostic)
	assert.Equal(t, stubGate(false), lifted.Feature)

	sts := newSingleContainerSTS()
	m := NewMutator(sts)
	conv := feature.Mutation[*Mutator](lifted)
	require.NoError(t, conv.ApplyIntent(m))
	require.NoError(t, m.Apply())

	assert.Empty(t, sts.Spec.Template.Spec.Containers[0].Env)
}

func TestLiftMutation_NilMutatePreserved(t *testing.T) {
	lifted := LiftMutation(feature.Mutation[primitives.WorkloadMutator]{Name: "nilmut"})
	assert.Nil(t, lifted.Mutate)

	conv := feature.Mutation[*Mutator](lifted)
	err := conv.ApplyIntent(NewMutator(newSingleContainerSTS()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutation handler of nilmut is nil")
}
