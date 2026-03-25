package replicaset

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
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
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

	m := NewMutator(rs)
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CHANGE", Value: "new"})
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "ADD", Value: "added"})
	m.RemoveContainerEnvVars([]string{"REMOVE", "NONEXISTENT"})

	err := m.Apply()
	require.NoError(t, err)

	env := rs.Spec.Template.Spec.Containers[0].Env
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
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
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

	m := NewMutator(rs)
	m.EnsureContainerArg("--change=new")
	m.EnsureContainerArg("--add")
	m.RemoveContainerArgs([]string{"--remove", "--nonexistent"})

	err := m.Apply()
	require.NoError(t, err)

	args := rs.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--keep")
	assert.Contains(t, args, "--change=old")
	assert.Contains(t, args, "--change=new")
	assert.Contains(t, args, "--add")
	assert.NotContains(t, args, "--remove")
}

func TestMutator_Replicas(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Replicas: ptr.To(int32(3)),
		},
	}

	m := NewMutator(rs)
	m.EnsureReplicas(5)

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, int32(5), *rs.Spec.Replicas)
}

func TestNewMutator(t *testing.T) {
	rs := &appsv1.ReplicaSet{}
	m := NewMutator(rs)
	assert.NotNil(t, m)
	assert.Equal(t, rs, m.current)
	require.Len(t, m.plans, 1, "NewMutator must create the initial feature plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the initial plan")
}

func TestNextFeature_AddsExactlyOnePlan(t *testing.T) {
	rs := &appsv1.ReplicaSet{}
	m := NewMutator(rs)

	require.Len(t, m.plans, 1, "NewMutator must create the initial plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the initial plan")

	m.NextFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestNextFeature_IsolatesFeaturePlans(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}
	m := NewMutator(rs)

	// Record mutations in the initial feature plan
	m.EnsureReplicas(3)
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v1"
		return nil
	})

	// Start a new feature and record different mutations
	m.NextFeature()
	m.EnsureReplicas(5)

	// First plan should have its edits, second plan should have its own
	assert.Len(t, m.plans[0].replicaSetSpecEdits, 1, "first plan should have one spec edit")
	assert.Len(t, m.plans[0].containerEdits, 1, "first plan should have one container edit")
	assert.Len(t, m.plans[1].replicaSetSpecEdits, 1, "second plan should have one spec edit")
	assert.Empty(t, m.plans[1].containerEdits, "second plan should have no container edits")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	rs := &appsv1.ReplicaSet{}
	m := NewMutator(rs)
	m.EnsureReplicas(3)

	require.NoError(t, m.Apply())
	assert.Len(t, m.plans, 1, "no extra plans should be created during Apply")
	assert.Equal(t, int32(3), *rs.Spec.Replicas)
}

func TestMutator_EditContainers(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
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

	m := NewMutator(rs)
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

	assert.Equal(t, "c1-image", rs.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "", rs.Spec.Template.Spec.Containers[1].Image)
	assert.Equal(t, "GLOBAL", rs.Spec.Template.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "GLOBAL", rs.Spec.Template.Spec.Containers[1].Env[0].Name)
}

func TestMutator_EditPodSpec(t *testing.T) {
	rs := &appsv1.ReplicaSet{}
	m := NewMutator(rs)
	m.EditPodSpec(func(e *editors.PodSpecEditor) error {
		e.Raw().ServiceAccountName = "my-sa"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "my-sa", rs.Spec.Template.Spec.ServiceAccountName)
}

func TestMutator_EditReplicaSetSpec(t *testing.T) {
	rs := &appsv1.ReplicaSet{}
	m := NewMutator(rs)
	m.EditReplicaSetSpec(func(e *editors.ReplicaSetSpecEditor) error {
		e.SetMinReadySeconds(10)
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, int32(10), rs.Spec.MinReadySeconds)
}

func TestMutator_EditMetadata(t *testing.T) {
	rs := &appsv1.ReplicaSet{}
	m := NewMutator(rs)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Labels = map[string]string{"rs": "label"}
		return nil
	})
	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Annotations = map[string]string{"pod": "ann"}
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "label", rs.Labels["rs"])
	assert.Equal(t, "ann", rs.Spec.Template.Annotations["pod"])
}

func TestMutator_Errors(t *testing.T) {
	rs := &appsv1.ReplicaSet{}
	m := NewMutator(rs)
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		return errors.New("boom")
	})

	err := m.Apply()
	assert.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestMutator_Order(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
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

	m := NewMutator(rs)
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
	m.EditReplicaSetSpec(func(_ *editors.ReplicaSetSpecEditor) error {
		order = append(order, "rsspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		order = append(order, "rsmeta")
		return nil
	})
	m.EnsureReplicas(3)

	err := m.Apply()
	require.NoError(t, err)

	expected := []string{"rsmeta", "rsspec", "podmeta", "podspec", "container"}
	assert.Equal(t, expected, order)
	assert.Equal(t, int32(3), *rs.Spec.Replicas)
}

func TestMutator_InitContainers(t *testing.T) {
	const newImage = "new-image"
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "old-image"},
					},
				},
			},
		},
	}

	m := NewMutator(rs)
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = newImage
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, newImage, rs.Spec.Template.Spec.InitContainers[0].Image)
}

func TestMutator_ContainerPresence(t *testing.T) {
	const newImage = "new-image"
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
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

	m := NewMutator(rs)
	// Replace
	m.EnsureContainer(corev1.Container{Name: "app", Image: "app-new-image"})
	// Remove
	m.RemoveContainer("sidecar")
	// Append
	m.EnsureContainer(corev1.Container{Name: "new-container", Image: newImage})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, rs.Spec.Template.Spec.Containers, 2)
	assert.Equal(t, "app", rs.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-new-image", rs.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "new-container", rs.Spec.Template.Spec.Containers[1].Name)
	assert.Equal(t, newImage, rs.Spec.Template.Spec.Containers[1].Image)
}

func TestMutator_InitContainerPresence(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "init-1", Image: "init-1-image"},
					},
				},
			},
		},
	}

	m := NewMutator(rs)
	m.EnsureInitContainer(corev1.Container{Name: "init-2", Image: "init-2-image"})
	m.RemoveInitContainers([]string{"init-1"})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, rs.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-2", rs.Spec.Template.Spec.InitContainers[0].Name)
}

func TestMutator_SelectorSnapshotSemantics(t *testing.T) {
	const appV2 = "app-v2"
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app-image"},
					},
				},
			},
		},
	}

	m := NewMutator(rs)
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

	assert.Equal(t, appV2, rs.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "app-image-updated", rs.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_Ordering_PresenceBeforeEdit(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(rs)
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
	require.Len(t, rs.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "edited-image", rs.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_NilSafety(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
	m := NewMutator(rs)
	// These should all be no-ops and not panic
	m.EditContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditContainers(selectors.AllContainers(), nil)
	m.EditPodSpec(nil)
	m.EditPodTemplateMetadata(nil)
	m.EditObjectMetadata(nil)
	m.EditReplicaSetSpec(nil)

	err := m.Apply()
	assert.NoError(t, err)
}

func TestMutator_CrossFeatureOrdering(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Replicas: ptr.To[int32](1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "v1"}},
				},
			},
		},
	}

	m := NewMutator(rs)

	// Feature A: sets replicas to 2, image to v2
	m.EnsureReplicas(2)
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2"
		return nil
	})

	// Feature B: sets replicas to 3, image to v3
	m.NextFeature()
	m.EnsureReplicas(3)
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v3"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	// Feature B should win
	assert.Equal(t, int32(3), *rs.Spec.Replicas)
	assert.Equal(t, "v3", rs.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_WithinFeatureCategoryOrdering(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "original-name"},
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(rs)
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
	m.EditReplicaSetSpec(func(_ *editors.ReplicaSetSpecEditor) error {
		executionOrder = append(executionOrder, "replicasetspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		executionOrder = append(executionOrder, "replicasetmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expectedOrder := []string{
		"replicasetmeta",
		"replicasetspec",
		"podmeta",
		"podspec",
		"container",
	}
	assert.Equal(t, expectedOrder, executionOrder)
}

func TestMutator_CrossFeatureVisibility(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
	}

	m := NewMutator(rs)

	// Feature A renames container
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Name = "app-v2"
		return nil
	})

	// Feature B selects by the new name - this should work!
	m.NextFeature()
	m.EditContainers(selectors.ContainerNamed("app-v2"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2-image"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, "app-v2", rs.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "v2-image", rs.Spec.Template.Spec.Containers[0].Image)
}

func TestMutator_InitContainer_OrderingAndSnapshots(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{},
				},
			},
		},
	}

	m := NewMutator(rs)
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

	require.Len(t, rs.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init-1-renamed", rs.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "v1-final", rs.Spec.Template.Spec.InitContainers[0].Image)
}
