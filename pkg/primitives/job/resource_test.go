package job

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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "test-ns",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "worker", Image: "busybox"},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidJob()).Build()
	require.NoError(t, err)
	assert.Equal(t, "batch/v1/Job/test-ns/test-job", res.Identity())
}

func TestResource_Object(t *testing.T) {
	job := newValidJob()
	res, err := NewBuilder(job).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*batchv1.Job)
	require.True(t, ok)
	assert.Equal(t, job.Name, got.Name)
	assert.Equal(t, job.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-job", job.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidJob()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*batchv1.Job)
	assert.Equal(t, "busybox", got.Spec.Template.Spec.Containers[0].Image)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidJob()
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

	got := obj.(*batchv1.Job)
	assert.Equal(t, "BAR", got.Spec.Template.Spec.Containers[0].Env[0].Value)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidJob()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditJobSpec(func(e *editors.JobSpecEditor) error {
					e.Raw().BackoffLimit = int32Ptr(5)
					return nil
				})
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditJobSpec(func(e *editors.JobSpecEditor) error {
					if e.Raw().BackoffLimit != nil && *e.Raw().BackoffLimit == 5 {
						e.Raw().BackoffLimit = int32Ptr(10)
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

	got := obj.(*batchv1.Job)
	require.NotNil(t, got.Spec.BackoffLimit)
	assert.Equal(t, int32(10), *got.Spec.BackoffLimit)
}

func TestResource_Mutate_CrossMutationSelectorSnapshot(t *testing.T) {
	desired := newValidJob()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-sidecar",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EnsureContainer(corev1.Container{
					Name:  "sidecar",
					Image: "sidecar:latest",
				})
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "configure-sidecar",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditContainers(selectors.ContainerNamed("sidecar"), func(e *editors.ContainerEditor) error {
					e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
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

	got := obj.(*batchv1.Job)
	// The sidecar container should exist and have the env var from the second mutation.
	var sidecar *corev1.Container
	for i := range got.Spec.Template.Spec.Containers {
		if got.Spec.Template.Spec.Containers[i].Name == "sidecar" {
			sidecar = &got.Spec.Template.Spec.Containers[i]
			break
		}
	}
	require.NotNil(t, sidecar, "sidecar container should be present")
	require.Len(t, sidecar.Env, 1)
	assert.Equal(t, "LOG_LEVEL", sidecar.Env[0].Name)
	assert.Equal(t, "debug", sidecar.Env[0].Value)
}

type mockHandlers struct {
	mock.Mock
}

func (m *mockHandlers) ConvergingStatus(op concepts.ConvergingOperation, j *batchv1.Job) (concepts.CompletionStatusWithReason, error) {
	args := m.Called(op, j)
	return args.Get(0).(concepts.CompletionStatusWithReason), args.Error(1)
}

func (m *mockHandlers) SuspensionStatus(j *batchv1.Job) (concepts.SuspensionStatusWithReason, error) {
	args := m.Called(j)
	return args.Get(0).(concepts.SuspensionStatusWithReason), args.Error(1)
}

func (m *mockHandlers) Suspend(mut *Mutator) error {
	args := m.Called(mut)
	return args.Error(0)
}

func (m *mockHandlers) DeleteOnSuspend(j *batchv1.Job) bool {
	args := m.Called(j)
	return args.Bool(0)
}

func TestResource_DeleteOnSuspend(t *testing.T) {
	job := newValidJob()

	t.Run("calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		m.On("DeleteOnSuspend", job).Return(false)

		res, err := NewBuilder(job).
			WithCustomSuspendDeletionDecision(m.DeleteOnSuspend).
			Build()
		require.NoError(t, err)
		assert.False(t, res.DeleteOnSuspend())
		m.AssertExpectations(t)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(job).Build()
		require.NoError(t, err)
		assert.True(t, res.DeleteOnSuspend())
	})
}

func TestResource_Suspend(t *testing.T) {
	job := newValidJob()

	t.Run("Suspend registers mutation and Mutate applies it using default handler", func(t *testing.T) {
		res, err := NewBuilder(job).Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		current := job.DeepCopy()
		err = res.Mutate(current)
		require.NoError(t, err)

		require.NotNil(t, current.Spec.Suspend)
		assert.True(t, *current.Spec.Suspend)
	})

	t.Run("Suspend uses custom mutation handler", func(t *testing.T) {
		m := &mockHandlers{}
		m.On("Suspend", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			mut := args.Get(0).(*Mutator)
			mut.EditJobSpec(func(e *editors.JobSpecEditor) error {
				e.Raw().BackoffLimit = int32Ptr(0)
				return nil
			})
		})

		res, err := NewBuilder(job).
			WithCustomSuspendMutation(m.Suspend).
			Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		current := job.DeepCopy()
		err = res.Mutate(current)
		require.NoError(t, err)

		m.AssertExpectations(t)
		require.NotNil(t, current.Spec.BackoffLimit)
		assert.Equal(t, int32(0), *current.Spec.BackoffLimit)
	})
}

func TestResource_SuspensionStatus(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "test-ns"},
		Spec: batchv1.JobSpec{
			Suspend: boolPtr(true),
		},
		Status: batchv1.JobStatus{
			Active: 0,
		},
	}

	t.Run("calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusSuspended := concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}
		m.On("SuspensionStatus", job).Return(statusSuspended, nil)

		res, err := NewBuilder(job).
			WithCustomSuspendStatus(m.SuspensionStatus).
			Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(job).Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})
}

func TestResource_ConvergingStatus(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "test-ns"},
		Status: batchv1.JobStatus{
			Active: 1,
		},
	}

	t.Run("calls handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusRunning := concepts.CompletionStatusWithReason{Status: concepts.CompletionStatusRunning}
		m.On("ConvergingStatus", concepts.ConvergingOperationUpdated, job).Return(statusRunning, nil)

		res, err := NewBuilder(job).
			WithCustomConvergeStatus(m.ConvergingStatus).
			Build()
		require.NoError(t, err)
		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.CompletionStatusRunning, status.Status)
	})

	t.Run("uses default", func(t *testing.T) {
		res, err := NewBuilder(job).Build()
		require.NoError(t, err)
		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		assert.Equal(t, concepts.CompletionStatusRunning, status.Status)
	})
}

func TestResource_ExtractData(t *testing.T) {
	job := newValidJob()

	extractedImage := ""
	res, err := NewBuilder(job).
		WithDataExtractor(func(j batchv1.Job) error {
			extractedImage = j.Spec.Template.Spec.Containers[0].Image
			return nil
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.NoError(t, err)
	assert.Equal(t, "busybox", extractedImage)
}

func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidJob()).
		WithDataExtractor(func(_ batchv1.Job) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}

func int32Ptr(i int32) *int32 {
	return &i
}
