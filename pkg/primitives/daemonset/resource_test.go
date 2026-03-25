package daemonset

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResource_Identity(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "test-ns",
		},
	}
	res, _ := NewBuilder(ds).Build()

	assert.Equal(t, "apps/v1/DaemonSet/test-ns/test-ds", res.Identity())
}

func TestResource_Object(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "test-ns",
		},
	}
	res, _ := NewBuilder(ds).Build()

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*appsv1.DaemonSet)
	require.True(t, ok)
	assert.Equal(t, ds.Name, got.Name)
	assert.Equal(t, ds.Namespace, got.Namespace)

	// Ensure it's a deep copy
	got.Name = "changed"
	assert.Equal(t, "test-ds", ds.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "web", Image: "nginx"},
					},
				},
			},
		},
	}

	res, _ := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "test-mutation",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EnsureContainerEnvVar(corev1.EnvVar{Name: "FOO", Value: "BAR"})
				return nil
			},
		}).
		Build()

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*appsv1.DaemonSet)
	assert.Equal(t, "test", got.Labels["app"])
	assert.Equal(t, "BAR", got.Spec.Template.Spec.Containers[0].Env[0].Value)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "v1"},
					},
				},
			},
		},
	}

	res, _ := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
					e.Raw().Image = "v2"
					return nil
				})
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
					if e.Raw().Image == "v2" {
						e.Raw().Image = "v3"
					}
					return nil
				})
				return nil
			},
		}).
		Build()

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*appsv1.DaemonSet)
	assert.Equal(t, "v3", got.Spec.Template.Spec.Containers[0].Image)
}

type mockHandlers struct {
	mock.Mock
}

func (m *mockHandlers) ConvergingStatus(op concepts.ConvergingOperation, d *appsv1.DaemonSet) (concepts.AliveStatusWithReason, error) {
	args := m.Called(op, d)
	return args.Get(0).(concepts.AliveStatusWithReason), args.Error(1)
}

func (m *mockHandlers) GraceStatus(d *appsv1.DaemonSet) (concepts.GraceStatusWithReason, error) {
	args := m.Called(d)
	return args.Get(0).(concepts.GraceStatusWithReason), args.Error(1)
}

func (m *mockHandlers) SuspensionStatus(d *appsv1.DaemonSet) (concepts.SuspensionStatusWithReason, error) {
	args := m.Called(d)
	return args.Get(0).(concepts.SuspensionStatusWithReason), args.Error(1)
}

func (m *mockHandlers) Suspend(mut *Mutator) error {
	args := m.Called(mut)
	return args.Error(0)
}

func (m *mockHandlers) DeleteOnSuspend(d *appsv1.DaemonSet) bool {
	args := m.Called(d)
	return args.Bool(0)
}

func TestResource_Status(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberReady:            2,
		},
	}

	t.Run("ConvergingStatus calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusReady := concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusHealthy}
		m.On("ConvergingStatus", concepts.ConvergingOperationUpdated, ds).Return(statusReady, nil)

		res, _ := NewBuilder(ds).
			WithCustomConvergeStatus(m.ConvergingStatus).
			Build()

		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.AliveConvergingStatusHealthy, status.Status)
	})

	t.Run("ConvergingStatus uses default", func(t *testing.T) {
		res, err := NewBuilder(ds).Build()
		require.NoError(t, err)
		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		assert.Equal(t, concepts.AliveConvergingStatusUpdating, status.Status)
	})

	t.Run("GraceStatus calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusReady := concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}
		m.On("GraceStatus", ds).Return(statusReady, nil)

		res, _ := NewBuilder(ds).
			WithCustomGraceStatus(m.GraceStatus).
			Build()

		status, err := res.GraceStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.GraceStatusHealthy, status.Status)
	})

	t.Run("GraceStatus uses default", func(t *testing.T) {
		res, err := NewBuilder(ds).Build()
		require.NoError(t, err)
		status, err := res.GraceStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, status.Status)
	})
}

func TestResource_DeleteOnSuspend(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	t.Run("calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		m.On("DeleteOnSuspend", ds).Return(false)

		res, err := NewBuilder(ds).
			WithCustomSuspendDeletionDecision(m.DeleteOnSuspend).
			Build()
		require.NoError(t, err)
		assert.False(t, res.DeleteOnSuspend())
		m.AssertExpectations(t)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(ds).Build()
		require.NoError(t, err)
		assert.True(t, res.DeleteOnSuspend())
	})
}

func TestResource_Suspend(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "nginx"},
					},
				},
			},
		},
	}

	t.Run("Suspend registers mutation and Mutate applies it using default handler", func(t *testing.T) {
		res, err := NewBuilder(ds).Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		obj, err := res.Object()
		require.NoError(t, err)
		require.NoError(t, res.Mutate(obj))

		got := obj.(*appsv1.DaemonSet)
		// Default suspend mutation is a no-op for DaemonSets
		assert.Equal(t, "nginx", got.Spec.Template.Spec.Containers[0].Image)
	})

	t.Run("Suspend uses custom mutation handler", func(t *testing.T) {
		m := &mockHandlers{}
		m.On("Suspend", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			mut := args.Get(0).(*Mutator)
			mut.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
				e.Raw().Image = "paused"
				return nil
			})
		})

		res, err := NewBuilder(ds).
			WithCustomSuspendMutation(m.Suspend).
			Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		obj, err := res.Object()
		require.NoError(t, err)
		require.NoError(t, res.Mutate(obj))

		got := obj.(*appsv1.DaemonSet)
		m.AssertExpectations(t)
		assert.Equal(t, "paused", got.Spec.Template.Spec.Containers[0].Image)
	})
}

func TestResource_SuspensionStatus(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	t.Run("calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusSuspended := concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}
		m.On("SuspensionStatus", ds).Return(statusSuspended, nil)

		res, err := NewBuilder(ds).
			WithCustomSuspendStatus(m.SuspensionStatus).
			Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(ds).Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})
}

func TestResource_ExtractData(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "web", Image: "nginx:latest"}},
				},
			},
		},
	}

	extractedImage := ""
	res, err := NewBuilder(ds).
		WithDataExtractor(func(d appsv1.DaemonSet) error {
			extractedImage = d.Spec.Template.Spec.Containers[0].Image
			return nil
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.NoError(t, err)
	assert.Equal(t, "nginx:latest", extractedImage)
}

