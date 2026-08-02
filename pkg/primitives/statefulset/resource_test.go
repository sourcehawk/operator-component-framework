package statefulset

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
	"k8s.io/utils/ptr"
)

func TestResource_Identity(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "test-ns",
		},
	}
	res, err := NewBuilder(sts).Build()
	require.NoError(t, err)

	assert.Equal(t, "apps/v1/StatefulSet/test-ns/test-sts", res.Identity())
}

func TestResource_Object(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "test-ns",
		},
	}
	res, err := NewBuilder(sts).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*appsv1.StatefulSet)
	require.True(t, ok)
	assert.Equal(t, sts.Name, got.Name)
	assert.Equal(t, sts.Namespace, got.Namespace)

	// Ensure it's a deep copy
	got.Name = "changed"
	assert.Equal(t, "test-sts", sts.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To(int32(3)),
			ServiceName: "test-svc",
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

	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "test-mutation",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EnsureContainerEnvVar(corev1.EnvVar{Name: "FOO", Value: "BAR"})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*appsv1.StatefulSet)
	assert.Equal(t, int32(3), *got.Spec.Replicas)
	assert.Equal(t, "test", got.Labels["app"])
	assert.Equal(t, "BAR", got.Spec.Template.Spec.Containers[0].Env[0].Value)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "v1"},
					},
				},
			},
		},
	}

	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewVersionGate("v1", nil).When(true),
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
			Feature: feature.NewVersionGate("v1", nil).When(true),
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
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*appsv1.StatefulSet)
	assert.Equal(t, "v3", got.Spec.Template.Spec.Containers[0].Image)
}

type mockHandlers struct {
	mock.Mock
}

func (m *mockHandlers) ConvergingStatus(op concepts.ConvergingOperation, s *appsv1.StatefulSet) (concepts.AliveStatusWithReason, error) {
	args := m.Called(op, s)
	return args.Get(0).(concepts.AliveStatusWithReason), args.Error(1)
}

func (m *mockHandlers) GraceStatus(s *appsv1.StatefulSet) (concepts.GraceStatusWithReason, error) {
	args := m.Called(s)
	return args.Get(0).(concepts.GraceStatusWithReason), args.Error(1)
}

func (m *mockHandlers) SuspensionStatus(s *appsv1.StatefulSet) (concepts.SuspensionStatusWithReason, error) {
	args := m.Called(s)
	return args.Get(0).(concepts.SuspensionStatusWithReason), args.Error(1)
}

func (m *mockHandlers) Suspend(mut *Mutator) error {
	args := m.Called(mut)
	return args.Error(0)
}

func (m *mockHandlers) DeleteOnSuspend(s *appsv1.StatefulSet) bool {
	args := m.Called(s)
	return args.Bool(0)
}

func TestResource_Status(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(int32(3)),
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 2,
			Replicas:      3,
		},
	}

	t.Run("ConvergingStatus calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusReady := concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusHealthy}
		m.On("ConvergingStatus", concepts.ConvergingOperationUpdated, sts).Return(statusReady, nil)

		res, err := NewBuilder(sts).
			WithCustomConvergeStatus(m.ConvergingStatus).
			Build()
		require.NoError(t, err)

		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.AliveConvergingStatusHealthy, status.Status)
	})

	t.Run("ConvergingStatus uses default", func(t *testing.T) {
		res, err := NewBuilder(sts).Build()
		require.NoError(t, err)
		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		assert.Equal(t, concepts.AliveConvergingStatusUpdating, status.Status)
	})

	t.Run("GraceStatus calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusReady := concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}
		m.On("GraceStatus", sts).Return(statusReady, nil)

		res, err := NewBuilder(sts).
			WithCustomGraceStatus(m.GraceStatus).
			Build()
		require.NoError(t, err)

		status, err := res.GraceStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.GraceStatusHealthy, status.Status)
	})

	t.Run("GraceStatus uses default", func(t *testing.T) {
		res, err := NewBuilder(sts).Build()
		require.NoError(t, err)
		status, err := res.GraceStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, status.Status)
	})
}

func TestResource_DeleteOnSuspend(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	t.Run("calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		m.On("DeleteOnSuspend", sts).Return(true)

		res, err := NewBuilder(sts).
			WithCustomSuspendDeletionDecision(m.DeleteOnSuspend).
			Build()
		require.NoError(t, err)
		assert.True(t, res.DeleteOnSuspend())
		m.AssertExpectations(t)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(sts).Build()
		require.NoError(t, err)
		assert.False(t, res.DeleteOnSuspend())
	})
}

func TestResource_Suspend(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(int32(3)),
		},
	}

	t.Run("Suspend registers mutation and Mutate applies it using default handler", func(t *testing.T) {
		res, err := NewBuilder(sts).Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		current := sts.DeepCopy()
		err = res.Mutate(current)
		require.NoError(t, err)

		assert.Equal(t, int32(0), *current.Spec.Replicas)
	})

	t.Run("Suspend uses custom mutation handler", func(t *testing.T) {
		m := &mockHandlers{}
		m.On("Suspend", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			mut := args.Get(0).(*Mutator)
			mut.EnsureReplicas(1)
		})

		res, err := NewBuilder(sts).
			WithCustomSuspendMutation(m.Suspend).
			Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		current := sts.DeepCopy()
		err = res.Mutate(current)
		require.NoError(t, err)

		m.AssertExpectations(t)
		assert.Equal(t, int32(1), *current.Spec.Replicas)
	})
}

func TestResource_SuspensionStatus(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status: appsv1.StatefulSetStatus{
			Replicas: 0,
		},
	}

	t.Run("calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusSuspended := concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}
		m.On("SuspensionStatus", sts).Return(statusSuspended, nil)

		res, err := NewBuilder(sts).
			WithCustomSuspendStatus(m.SuspensionStatus).
			Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(sts).Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})
}
