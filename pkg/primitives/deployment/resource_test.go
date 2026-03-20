package deployment

import (
	"errors"
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
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "test-ns",
		},
	}
	res, _ := NewBuilder(deploy).Build()

	assert.Equal(t, "apps/v1/Deployment/test-ns/test-deploy", res.Identity())
}

func TestResource_Object(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "test-ns",
		},
	}
	res, _ := NewBuilder(deploy).Build()

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*appsv1.Deployment)
	require.True(t, ok)
	assert.Equal(t, deploy.Name, got.Name)
	assert.Equal(t, deploy.Namespace, got.Namespace)

	// Ensure it's a deep copy
	got.Name = "changed"
	assert.Equal(t, "test-deploy", deploy.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
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
		WithMutation(feature.Mutation[*Mutator]{
			Name:    "test-mutation",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EnsureContainerEnvVar(corev1.EnvVar{Name: "FOO", Value: "BAR"})
				return nil
			},
		}).
		Build()

	current := &appsv1.Deployment{}
	err := res.Mutate(current)
	require.NoError(t, err)

	assert.Equal(t, int32(3), *current.Spec.Replicas)
	assert.Equal(t, "test", current.Labels["app"])
	assert.Equal(t, "BAR", current.Spec.Template.Spec.Containers[0].Env[0].Value)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
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
		WithMutation(feature.Mutation[*Mutator]{
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
		WithMutation(feature.Mutation[*Mutator]{
			Name:    "feature-b",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				// This should see image "v2" if BeginFeature() is working correctly between mutations
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

	current := &appsv1.Deployment{}
	err := res.Mutate(current)
	require.NoError(t, err)

	assert.Equal(t, "v3", current.Spec.Template.Spec.Containers[0].Image)
}

type mockHandlers struct {
	mock.Mock
}

func (m *mockHandlers) ConvergingStatus(op concepts.ConvergingOperation, d *appsv1.Deployment) (concepts.AliveStatusWithReason, error) {
	args := m.Called(op, d)
	return args.Get(0).(concepts.AliveStatusWithReason), args.Error(1)
}

func (m *mockHandlers) GraceStatus(d *appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
	args := m.Called(d)
	return args.Get(0).(concepts.GraceStatusWithReason), args.Error(1)
}

func (m *mockHandlers) SuspensionStatus(d *appsv1.Deployment) (concepts.SuspensionStatusWithReason, error) {
	args := m.Called(d)
	return args.Get(0).(concepts.SuspensionStatusWithReason), args.Error(1)
}

func (m *mockHandlers) Suspend(mut *Mutator) error {
	args := m.Called(mut)
	return args.Error(0)
}

func (m *mockHandlers) DeleteOnSuspend(d *appsv1.Deployment) bool {
	args := m.Called(d)
	return args.Bool(0)
}

func TestResource_Status(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 2,
			Replicas:      3,
		},
	}

	t.Run("ConvergingStatus calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusReady := concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusHealthy}
		m.On("ConvergingStatus", concepts.ConvergingOperationUpdated, deploy).Return(statusReady, nil)

		res, _ := NewBuilder(deploy).
			WithCustomConvergeStatus(m.ConvergingStatus).
			Build()

		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.AliveConvergingStatusHealthy, status.Status)
	})

	t.Run("ConvergingStatus uses default", func(t *testing.T) {
		res, err := NewBuilder(deploy).Build()
		require.NoError(t, err)
		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		assert.Equal(t, concepts.AliveConvergingStatusUpdating, status.Status)
	})

	t.Run("GraceStatus calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusReady := concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}
		m.On("GraceStatus", deploy).Return(statusReady, nil)

		res, _ := NewBuilder(deploy).
			WithCustomGraceStatus(m.GraceStatus).
			Build()

		status, err := res.GraceStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.GraceStatusHealthy, status.Status)
	})

	t.Run("GraceStatus uses default", func(t *testing.T) {
		res, err := NewBuilder(deploy).Build()
		require.NoError(t, err)
		status, err := res.GraceStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, status.Status)
	})
}

func TestResource_DeleteOnSuspend(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	t.Run("calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		m.On("DeleteOnSuspend", deploy).Return(true)

		res, err := NewBuilder(deploy).
			WithCustomSuspendDeletionDecision(m.DeleteOnSuspend).
			Build()
		require.NoError(t, err)
		assert.True(t, res.DeleteOnSuspend())
		m.AssertExpectations(t)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(deploy).Build()
		require.NoError(t, err)
		assert.False(t, res.DeleteOnSuspend())
	})
}

func TestResource_Suspend(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
		},
	}

	t.Run("Suspend registers mutation and Mutate applies it using default handler", func(t *testing.T) {
		res, err := NewBuilder(deploy).Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		current := deploy.DeepCopy()
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

		res, err := NewBuilder(deploy).
			WithCustomSuspendMutation(m.Suspend).
			Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		current := deploy.DeepCopy()
		err = res.Mutate(current)
		require.NoError(t, err)

		m.AssertExpectations(t)
		assert.Equal(t, int32(1), *current.Spec.Replicas)
	})
}

func TestResource_SuspensionStatus(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status: appsv1.DeploymentStatus{
			Replicas: 0,
		},
	}

	t.Run("calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusSuspended := concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}
		m.On("SuspensionStatus", deploy).Return(statusSuspended, nil)

		res, err := NewBuilder(deploy).
			WithCustomSuspendStatus(m.SuspensionStatus).
			Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(deploy).Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})
}

func TestResource_ExtractData(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "web", Image: "nginx:latest"}},
				},
			},
		},
	}

	extractedImage := ""
	res, err := NewBuilder(deploy).
		WithDataExtractor(func(d appsv1.Deployment) error {
			extractedImage = d.Spec.Template.Spec.Containers[0].Image
			return nil
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.NoError(t, err)
	assert.Equal(t, "nginx:latest", extractedImage)
}

func TestResource_CustomFieldApplicator(t *testing.T) {
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
		},
	}

	applicatorCalled := false
	res, _ := NewBuilder(desired).
		WithCustomFieldApplicator(func(current *appsv1.Deployment, desired *appsv1.Deployment) error {
			applicatorCalled = true
			current.Name = desired.Name
			current.Namespace = desired.Namespace
			// Only apply replicas, ignore labels
			current.Spec.Replicas = desired.Spec.Replicas
			return nil
		}).
		Build()

	current := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"external": "label"},
		},
	}
	err := res.Mutate(current)
	require.NoError(t, err)

	assert.True(t, applicatorCalled)
	assert.Equal(t, int32(3), *current.Spec.Replicas)
	assert.Equal(t, "label", current.Labels["external"], "External label should be preserved")
	assert.NotContains(t, current.Labels, "app", "Desired label should NOT be applied by custom applicator")

	t.Run("returns error", func(t *testing.T) {
		res, _ := NewBuilder(desired).
			WithCustomFieldApplicator(func(_ *appsv1.Deployment, _ *appsv1.Deployment) error {
				return errors.New("applicator error")
			}).
			Build()

		err := res.Mutate(&appsv1.Deployment{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "applicator error")
	})
}
