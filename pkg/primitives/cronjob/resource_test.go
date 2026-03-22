package cronjob

import (
	"errors"
	"testing"
	"time"

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

func newValidCronJob() *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cj",
			Namespace: "test-ns",
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 2 * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "worker", Image: "worker:latest"},
							},
							RestartPolicy: corev1.RestartPolicyOnFailure,
						},
					},
				},
			},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidCronJob()).Build()
	require.NoError(t, err)
	assert.Equal(t, "batch/v1/CronJob/test-ns/test-cj", res.Identity())
}

func TestResource_Object(t *testing.T) {
	cj := newValidCronJob()
	res, err := NewBuilder(cj).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*batchv1.CronJob)
	require.True(t, ok)
	assert.Equal(t, cj.Name, got.Name)
	assert.Equal(t, cj.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-cj", cj.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidCronJob()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	current := &batchv1.CronJob{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, "0 2 * * *", current.Spec.Schedule)
	assert.Equal(t, "worker", current.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Name)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidCronJob()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-env",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EnsureContainerEnvVar(corev1.EnvVar{Name: "FOO", Value: "BAR"})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	current := &batchv1.CronJob{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, "0 2 * * *", current.Spec.Schedule)
	assert.Equal(t, "BAR", current.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env[0].Value)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidCronJob()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditContainers(selectors.ContainerNamed("worker"), func(e *editors.ContainerEditor) error {
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
				m.EditContainers(selectors.ContainerNamed("worker"), func(e *editors.ContainerEditor) error {
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

	current := &batchv1.CronJob{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, "v3", current.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image)
}

func TestResource_Mutate_CustomFieldApplicator(t *testing.T) {
	desired := newValidCronJob()
	desired.Labels = map[string]string{"app": "test"}

	applicatorCalled := false
	res, err := NewBuilder(desired).
		WithCustomFieldApplicator(func(current, d *batchv1.CronJob) error {
			applicatorCalled = true
			current.Name = d.Name
			current.Namespace = d.Namespace
			current.Spec.Schedule = d.Spec.Schedule
			return nil
		}).
		Build()
	require.NoError(t, err)

	current := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"external": "label"},
		},
	}
	require.NoError(t, res.Mutate(current))

	assert.True(t, applicatorCalled)
	assert.Equal(t, "0 2 * * *", current.Spec.Schedule)
	assert.Equal(t, "label", current.Labels["external"])
	assert.NotContains(t, current.Labels, "app")

	t.Run("returns error", func(t *testing.T) {
		res, err := NewBuilder(newValidCronJob()).
			WithCustomFieldApplicator(func(_, _ *batchv1.CronJob) error {
				return errors.New("applicator error")
			}).
			Build()
		require.NoError(t, err)

		err = res.Mutate(&batchv1.CronJob{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "applicator error")
	})
}

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	current := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cj",
			Namespace:       "test-ns",
			ResourceVersion: "12345",
			UID:             "abc-def",
			Generation:      3,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Pod", Name: "other-owner", UID: "other-uid"},
			},
			Finalizers: []string{"finalizer.example.com"},
		},
	}
	desired := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cj",
			Namespace: "test-ns",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 2 * * *",
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec and labels are applied
	assert.Equal(t, "0 2 * * *", current.Spec.Schedule)
	assert.Equal(t, "test", current.Labels["app"])

	// Server-managed fields are preserved
	assert.Equal(t, "12345", current.ResourceVersion)
	assert.Equal(t, "abc-def", string(current.UID))
	assert.Equal(t, int64(3), current.Generation)

	// Shared-controller fields are preserved
	assert.Len(t, current.OwnerReferences, 1)
	assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)
}

type mockHandlers struct {
	mock.Mock
}

func (m *mockHandlers) ConvergingStatus(op concepts.ConvergingOperation, cj *batchv1.CronJob) (concepts.OperationalStatusWithReason, error) {
	args := m.Called(op, cj)
	return args.Get(0).(concepts.OperationalStatusWithReason), args.Error(1)
}

func (m *mockHandlers) SuspensionStatus(cj *batchv1.CronJob) (concepts.SuspensionStatusWithReason, error) {
	args := m.Called(cj)
	return args.Get(0).(concepts.SuspensionStatusWithReason), args.Error(1)
}

func (m *mockHandlers) Suspend(mut *Mutator) error {
	args := m.Called(mut)
	return args.Error(0)
}

func (m *mockHandlers) DeleteOnSuspend(cj *batchv1.CronJob) bool {
	args := m.Called(cj)
	return args.Bool(0)
}

func TestResource_ConvergingStatus(t *testing.T) {
	cj := newValidCronJob()

	t.Run("calls custom handler", func(t *testing.T) {
		m := &mockHandlers{}
		statusOp := concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusOperational}
		m.On("ConvergingStatus", concepts.ConvergingOperationUpdated, cj).Return(statusOp, nil)

		res, err := NewBuilder(cj).
			WithCustomOperationalStatus(m.ConvergingStatus).
			Build()
		require.NoError(t, err)

		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.OperationalStatusOperational, status.Status)
	})

	t.Run("uses default - pending", func(t *testing.T) {
		res, err := NewBuilder(cj).Build()
		require.NoError(t, err)
		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		assert.Equal(t, concepts.OperationalStatusPending, status.Status)
	})

	t.Run("uses default - operational", func(t *testing.T) {
		cjScheduled := newValidCronJob()
		now := metav1.NewTime(time.Now())
		cjScheduled.Status.LastScheduleTime = &now

		res, err := NewBuilder(cjScheduled).Build()
		require.NoError(t, err)
		status, err := res.ConvergingStatus(concepts.ConvergingOperationUpdated)
		require.NoError(t, err)
		assert.Equal(t, concepts.OperationalStatusOperational, status.Status)
	})
}

func TestResource_DeleteOnSuspend(t *testing.T) {
	cj := newValidCronJob()

	t.Run("calls custom handler", func(t *testing.T) {
		m := &mockHandlers{}
		m.On("DeleteOnSuspend", cj).Return(true)

		res, err := NewBuilder(cj).
			WithCustomSuspendDeletionDecision(m.DeleteOnSuspend).
			Build()
		require.NoError(t, err)
		assert.True(t, res.DeleteOnSuspend())
		m.AssertExpectations(t)
	})

	t.Run("uses default - false", func(t *testing.T) {
		res, err := NewBuilder(cj).Build()
		require.NoError(t, err)
		assert.False(t, res.DeleteOnSuspend())
	})
}

func TestResource_Suspend(t *testing.T) {
	cj := newValidCronJob()

	t.Run("registers mutation and Mutate applies it using default handler", func(t *testing.T) {
		res, err := NewBuilder(cj).Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		current := cj.DeepCopy()
		err = res.Mutate(current)
		require.NoError(t, err)

		require.NotNil(t, current.Spec.Suspend)
		assert.True(t, *current.Spec.Suspend)
	})

	t.Run("uses custom mutation handler", func(t *testing.T) {
		m := &mockHandlers{}
		m.On("Suspend", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			mut := args.Get(0).(*Mutator)
			mut.EditCronJobSpec(func(e *editors.CronJobSpecEditor) error {
				suspend := true
				e.Raw().Suspend = &suspend
				e.SetSchedule("0 0 31 2 *") // effectively disable
				return nil
			})
		})

		res, err := NewBuilder(cj).
			WithCustomSuspendMutation(m.Suspend).
			Build()
		require.NoError(t, err)
		err = res.Suspend()
		require.NoError(t, err)

		current := cj.DeepCopy()
		err = res.Mutate(current)
		require.NoError(t, err)

		m.AssertExpectations(t)
		require.NotNil(t, current.Spec.Suspend)
		assert.True(t, *current.Spec.Suspend)
		assert.Equal(t, "0 0 31 2 *", current.Spec.Schedule)
	})
}

func TestResource_SuspensionStatus(t *testing.T) {
	t.Run("calls custom handler", func(t *testing.T) {
		cj := newValidCronJob()
		m := &mockHandlers{}
		statusSuspended := concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}
		m.On("SuspensionStatus", cj).Return(statusSuspended, nil)

		res, err := NewBuilder(cj).
			WithCustomSuspendStatus(m.SuspensionStatus).
			Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		m.AssertExpectations(t)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})

	t.Run("uses default - suspending when not yet suspended", func(t *testing.T) {
		cj := newValidCronJob()
		res, err := NewBuilder(cj).Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspending, status.Status)
	})

	t.Run("uses default - suspended when suspend=true and no active jobs", func(t *testing.T) {
		cj := newValidCronJob()
		suspend := true
		cj.Spec.Suspend = &suspend

		res, err := NewBuilder(cj).Build()
		require.NoError(t, err)
		status, err := res.SuspensionStatus()
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})
}

func TestResource_ExtractData(t *testing.T) {
	cj := newValidCronJob()

	var extractedImage string
	res, err := NewBuilder(cj).
		WithDataExtractor(func(c batchv1.CronJob) error {
			extractedImage = c.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image
			return nil
		}).
		Build()
	require.NoError(t, err)

	require.NoError(t, res.ExtractData())
	assert.Equal(t, "worker:latest", extractedImage)
}

func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidCronJob()).
		WithDataExtractor(func(_ batchv1.CronJob) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}
