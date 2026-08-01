package pod

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "nginx:latest"},
			},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidPod()).Build()
	require.NoError(t, err)
	assert.Equal(t, "v1/Pod/test-ns/test-pod", res.Identity())
}

func TestResource_Object(t *testing.T) {
	pod := newValidPod()
	res, err := NewBuilder(pod).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*corev1.Pod)
	require.True(t, ok)
	assert.Equal(t, pod.Name, got.Name)
	assert.Equal(t, pod.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-pod", pod.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidPod()
	desired.Labels = map[string]string{"app": "test"}

	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-env",
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

	got := obj.(*corev1.Pod)
	assert.Equal(t, "test", got.Labels["app"])
	assert.Equal(t, "BAR", got.Spec.Containers[0].Env[0].Value)
}

func TestResource_Mutate_DeepCopySemantics(t *testing.T) {
	desired := newValidPod()
	desired.Labels = map[string]string{"app": "test"}

	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	// Modifying the mutated object's labels must not affect the desired object.
	got := obj.(*corev1.Pod)
	got.Labels["app"] = "modified"
	assert.Equal(t, "test", desired.Labels["app"])
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidPod()

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

	got := obj.(*corev1.Pod)
	assert.Equal(t, "v3", got.Spec.Containers[0].Image)
}

func TestResource_Mutate_DisabledFeatureSkipped(t *testing.T) {
	desired := newValidPod()

	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "disabled",
			Feature: feature.NewVersionGate("v1", nil).When(false),
			Mutate: func(m *Mutator) error {
				m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHOULD_NOT_EXIST", Value: "true"})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*corev1.Pod)
	assert.Empty(t, got.Spec.Containers[0].Env)
}

type podMockHandlers struct {
	mock.Mock
}

func (m *podMockHandlers) ConvergingStatus(op concepts.ConvergingOperation, p *corev1.Pod) (concepts.AliveStatusWithReason, error) {
	args := m.Called(op, p)
	return args.Get(0).(concepts.AliveStatusWithReason), args.Error(1)
}

func (m *podMockHandlers) GraceStatus(p *corev1.Pod) (concepts.GraceStatusWithReason, error) {
	args := m.Called(p)
	return args.Get(0).(concepts.GraceStatusWithReason), args.Error(1)
}

func (m *podMockHandlers) SuspensionStatus(p *corev1.Pod) (concepts.SuspensionStatusWithReason, error) {
	args := m.Called(p)
	return args.Get(0).(concepts.SuspensionStatusWithReason), args.Error(1)
}

func (m *podMockHandlers) Suspend(mut *Mutator) error {
	args := m.Called(mut)
	return args.Error(0)
}

func (m *podMockHandlers) DeleteOnSuspend(p *corev1.Pod) bool {
	args := m.Called(p)
	return args.Bool(0)
}

func TestResource_ConvergingStatus(t *testing.T) {
	pod := newValidPod()
	pod.Status.Phase = corev1.PodRunning
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{Name: "app", Ready: true},
	}

	t.Run("calls custom handler", func(t *testing.T) {
		m := &podMockHandlers{}
		statusReady := concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusHealthy}
		m.On("ConvergingStatus", concepts.ConvergingOperationUpdated, pod).Return(statusReady, nil)

		res, err := NewBuilder(pod).
			WithCustomConvergeStatus(m.ConvergingStatus).
			Build()
		require.NoError(t, err)

		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.AliveConvergingStatusHealthy, status.Status)
	})

	t.Run("uses default handler", func(t *testing.T) {
		res, err := NewBuilder(pod).Build()
		require.NoError(t, err)
		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		assert.Equal(t, concepts.AliveConvergingStatusHealthy, status.Status)
	})
}

func TestResource_GraceStatus(t *testing.T) {
	pod := newValidPod()
	pod.Status.Phase = corev1.PodRunning
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{Name: "app", Ready: true},
	}

	t.Run("calls custom handler", func(t *testing.T) {
		m := &podMockHandlers{}
		statusReady := concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}
		m.On("GraceStatus", pod).Return(statusReady, nil)

		res, err := NewBuilder(pod).
			WithCustomGraceStatus(m.GraceStatus).
			Build()
		require.NoError(t, err)

		status, err := res.GraceStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.GraceStatusHealthy, status.Status)
	})

	t.Run("uses default handler", func(t *testing.T) {
		res, err := NewBuilder(pod).Build()
		require.NoError(t, err)
		status, err := res.GraceStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, status.Status)
	})
}

func TestResource_DeleteOnSuspend(t *testing.T) {
	pod := newValidPod()

	t.Run("calls custom handler", func(t *testing.T) {
		m := &podMockHandlers{}
		m.On("DeleteOnSuspend", pod).Return(false)

		res, err := NewBuilder(pod).
			WithCustomSuspendDeletionDecision(m.DeleteOnSuspend).
			Build()
		require.NoError(t, err)
		assert.False(t, res.DeleteOnSuspend())
		m.AssertExpectations(t)
	})

	t.Run("uses default (true for pods)", func(t *testing.T) {
		res, err := NewBuilder(pod).Build()
		require.NoError(t, err)
		assert.True(t, res.DeleteOnSuspend())
	})
}

func TestResource_Suspend(t *testing.T) {
	pod := newValidPod()

	t.Run("registers mutation and Mutate applies it using default handler", func(t *testing.T) {
		res, err := NewBuilder(pod).Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		obj, err := res.Object()
		require.NoError(t, err)
		err = res.Mutate(obj)
		require.NoError(t, err)
		// Default suspend mutation is a no-op for pods (they are deleted instead).
		// Just verify Mutate succeeds.
	})

	t.Run("uses custom suspend mutation handler", func(t *testing.T) {
		m := &podMockHandlers{}
		m.On("Suspend", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			mut := args.Get(0).(*Mutator)
			mut.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("suspended", "true")
				return nil
			})
		})

		res, err := NewBuilder(pod).
			WithCustomSuspendMutation(m.Suspend).
			Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		obj, err := res.Object()
		require.NoError(t, err)
		err = res.Mutate(obj)
		require.NoError(t, err)

		m.AssertExpectations(t)
		got := obj.(*corev1.Pod)
		assert.Equal(t, "true", got.Labels["suspended"])
	})
}

func TestResource_SuspensionStatus(t *testing.T) {
	pod := newValidPod()

	t.Run("calls custom handler", func(t *testing.T) {
		m := &podMockHandlers{}
		statusSuspended := concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}
		m.On("SuspensionStatus", pod).Return(statusSuspended, nil)

		res, err := NewBuilder(pod).
			WithCustomSuspendStatus(m.SuspensionStatus).
			Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(pod).Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
		assert.Equal(t, "Pod deleted on suspend", status.Reason)
	})
}
