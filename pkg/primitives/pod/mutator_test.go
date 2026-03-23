package pod

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewMutator(t *testing.T) {
	pod := &corev1.Pod{}
	m := NewMutator(pod)
	assert.NotNil(t, m)
	assert.Equal(t, pod, m.current)
}

func TestMutator_EnvVars(t *testing.T) {
	pod := &corev1.Pod{
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
	}

	m := NewMutator(pod)
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "CHANGE", Value: "new"})
	m.EnsureContainerEnvVar(corev1.EnvVar{Name: "ADD", Value: "added"})
	m.RemoveContainerEnvVars([]string{"REMOVE", "NONEXISTENT"})

	err := m.Apply()
	require.NoError(t, err)

	env := pod.Spec.Containers[0].Env
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
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Args: []string{"--keep", "--change=old", "--remove"},
				},
			},
		},
	}

	m := NewMutator(pod)
	m.EnsureContainerArg("--change=new")
	m.EnsureContainerArg("--add")
	m.RemoveContainerArgs([]string{"--remove", "--nonexistent"})

	err := m.Apply()
	require.NoError(t, err)

	args := pod.Spec.Containers[0].Args
	assert.Contains(t, args, "--keep")
	assert.Contains(t, args, "--change=old")
	assert.Contains(t, args, "--change=new")
	assert.Contains(t, args, "--add")
	assert.NotContains(t, args, "--remove")
}

func TestMutator_EditContainers(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "c1"},
				{Name: "c2"},
			},
		},
	}

	m := NewMutator(pod)
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

	assert.Equal(t, "c1-image", pod.Spec.Containers[0].Image)
	assert.Equal(t, "", pod.Spec.Containers[1].Image)
	assert.Equal(t, "GLOBAL", pod.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "GLOBAL", pod.Spec.Containers[1].Env[0].Name)
}

func TestMutator_EditPodSpec(t *testing.T) {
	pod := &corev1.Pod{}
	m := NewMutator(pod)
	m.EditPodSpec(func(e *editors.PodSpecEditor) error {
		e.Raw().ServiceAccountName = "my-sa"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "my-sa", pod.Spec.ServiceAccountName)
}

func TestMutator_EditMetadata(t *testing.T) {
	pod := &corev1.Pod{}
	m := NewMutator(pod)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.Raw().Labels = map[string]string{"pod": "label"}
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)
	assert.Equal(t, "label", pod.Labels["pod"])
}

func TestMutator_Errors(t *testing.T) {
	pod := &corev1.Pod{}
	m := NewMutator(pod)
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		return errors.New("boom")
	})

	err := m.Apply()
	assert.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestMutator_Order(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main"}},
		},
	}

	var order []string

	m := NewMutator(pod)
	// Register in reverse order of expected execution
	m.EditContainers(selectors.AllContainers(), func(_ *editors.ContainerEditor) error {
		order = append(order, "container")
		return nil
	})
	m.EditPodSpec(func(_ *editors.PodSpecEditor) error {
		order = append(order, "podspec")
		return nil
	})
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		order = append(order, "podmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expected := []string{"podmeta", "podspec", "container"}
	assert.Equal(t, expected, order)
}

func TestMutator_InitContainers(t *testing.T) {
	const newImage = "new-image"
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "init-1", Image: "old-image"},
			},
		},
	}

	m := NewMutator(pod)
	m.EditInitContainers(selectors.ContainerNamed("init-1"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = newImage
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	assert.Equal(t, newImage, pod.Spec.InitContainers[0].Image)
}

func TestMutator_ContainerPresence(t *testing.T) {
	const newImage = "new-image"
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "app-image"},
				{Name: "sidecar", Image: "sidecar-image"},
			},
		},
	}

	m := NewMutator(pod)
	// Replace
	m.EnsureContainer(corev1.Container{Name: "app", Image: "app-new-image"})
	// Remove
	m.RemoveContainer("sidecar")
	// Append
	m.EnsureContainer(corev1.Container{Name: "new-container", Image: newImage})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, pod.Spec.Containers, 2)

	assert.Equal(t, "app", pod.Spec.Containers[0].Name)
	assert.Equal(t, "app-new-image", pod.Spec.Containers[0].Image)

	assert.Equal(t, "new-container", pod.Spec.Containers[1].Name)
	assert.Equal(t, newImage, pod.Spec.Containers[1].Image)
}

func TestMutator_InitContainerPresence(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "init-1", Image: "init-1-image"},
			},
		},
	}

	m := NewMutator(pod)
	m.EnsureInitContainer(corev1.Container{Name: "init-2", Image: "init-2-image"})
	m.RemoveInitContainers([]string{"init-1"})

	err := m.Apply()
	require.NoError(t, err)

	require.Len(t, pod.Spec.InitContainers, 1)

	assert.Equal(t, "init-2", pod.Spec.InitContainers[0].Name)
}

func TestMutator_SelectorSnapshotSemantics(t *testing.T) {
	const appV2 = "app-v2"
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "app-image"},
			},
		},
	}

	m := NewMutator(pod)

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

	assert.Equal(t, appV2, pod.Spec.Containers[0].Name)
	assert.Equal(t, "app-image-updated", pod.Spec.Containers[0].Image)
}

func TestMutator_Ordering_PresenceBeforeEdit(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{},
		},
	}

	m := NewMutator(pod)

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
	require.Len(t, pod.Spec.Containers, 1)

	assert.Equal(t, "edited-image", pod.Spec.Containers[0].Image)
}

func TestMutator_NilSafety(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main"}},
		},
	}
	m := NewMutator(pod)

	// These should all be no-ops and not panic
	m.EditContainers(nil, func(_ *editors.ContainerEditor) error { return nil })
	m.EditContainers(selectors.AllContainers(), nil)
	m.EditPodSpec(nil)
	m.EditObjectMetadata(nil)

	err := m.Apply()
	assert.NoError(t, err)
}

func TestMutator_CrossFeatureOrdering(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "v1"}},
		},
	}

	m := NewMutator(pod)

	// Feature A: sets image to v2
	m.BeginFeature()
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v2"
		return nil
	})

	// Feature B: sets image to v3
	m.BeginFeature()
	m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
		e.Raw().Image = "v3"
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	// Feature B should win
	assert.Equal(t, "v3", pod.Spec.Containers[0].Image)
}

func TestMutator_WithinFeatureCategoryOrdering(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "original-name"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}

	m := NewMutator(pod)

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
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		executionOrder = append(executionOrder, "podmeta")
		return nil
	})

	err := m.Apply()
	require.NoError(t, err)

	expectedOrder := []string{
		"podmeta",
		"podspec",
		"container",
	}
	assert.Equal(t, expectedOrder, executionOrder)
}

func TestMutator_CrossFeatureVisibility(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}

	m := NewMutator(pod)

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

	assert.Equal(t, "app-v2", pod.Spec.Containers[0].Name)
	assert.Equal(t, "v2-image", pod.Spec.Containers[0].Image)
}

func TestMutator_InitContainer_OrderingAndSnapshots(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{},
		},
	}

	m := NewMutator(pod)

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

	require.Len(t, pod.Spec.InitContainers, 1)
	assert.Equal(t, "init-1-renamed", pod.Spec.InitContainers[0].Name)
	assert.Equal(t, "v1-final", pod.Spec.InitContainers[0].Image)
}
