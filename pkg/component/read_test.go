package component

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadResources(t *testing.T) {
	var (
		scheme    = setupScheme()
		namespace = "test-namespace"
		owner     = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-owner",
				Namespace: namespace,
			},
		}
		fakeClient       = fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
		reconcileContext = setupReconcileContext(scheme, owner, fakeClient)
		ctx              = t.Context()
	)

	t.Run("should successfully read multiple resources with different concepts", func(t *testing.T) {
		// Given
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: namespace,
			},
		}
		require.NoError(t, fakeClient.Create(ctx, cm))

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: namespace,
			},
		}
		require.NoError(t, fakeClient.Create(ctx, secret))

		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: namespace,
			},
		}
		require.NoError(t, fakeClient.Create(ctx, service))

		job := &corev1.Pod{ // Using Pod as a generic object for Job
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: namespace,
			},
		}
		require.NoError(t, fakeClient.Create(ctx, job))

		// Resource 1: not Alive/Operational/Completable
		resource1 := &MockResource{}
		resource1.On("Object").Return(cm, nil)

		// Resource 2: Alive
		resource2 := &MockAliveResource{}
		resource2.On("Object").Return(secret, nil)
		resource2.On("ConvergingStatus", concepts.ConvergingOperationNone).Return(concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusHealthy,
			Reason: "Secret is healthy",
		}, nil)

		// Resource 3: Operational
		resource3 := &MockOperationalResource{}
		resource3.On("Object").Return(service, nil)
		resource3.On("ConvergingStatus", concepts.ConvergingOperationNone).Return(concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusOperational,
			Reason: "Service is operational",
		}, nil)

		// Resource 4: Completable
		resource4 := &MockCompletableResource{}
		resource4.On("Object").Return(job, nil)
		resource4.On("ConvergingStatus", concepts.ConvergingOperationNone).Return(concepts.CompletionStatusWithReason{
			Status: concepts.CompletionStatusCompleted,
			Reason: "Job completed",
		}, nil)

		// When
		results, err := readResources(ctx, reconcileContext, []Resource{resource1, resource2, resource3, resource4})

		// Then
		require.NoError(t, err)
		assert.Len(t, results, 3) // Alive, Operational, Completable

		assert.Equal(t, convergingStatusAliveHealthy, results[0].Status.Status)

		assert.Equal(t, convergingStatusOperationalOperational, results[1].Status.Status)

		assert.Equal(t, convergingStatusCompletableCompleted, results[2].Status.Status)

		resource1.AssertExpectations(t)
		resource2.AssertExpectations(t)
		resource3.AssertExpectations(t)
		resource4.AssertExpectations(t)
	})

	t.Run("should return error if resource.Object() fails", func(t *testing.T) {
		// Given
		resource := &MockResource{}
		resource.On("Object").Return(nil, errors.New("failed to get object"))
		resource.On("Identity").Return("v1/ConfigMap/failed-resource")

		// When
		results, err := readResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "failed to retrieve read-only object from resource v1/ConfigMap/failed-resource: failed to get object")

		resource.AssertExpectations(t)
	})

	t.Run("should return error and update status if client.Get() fails", func(t *testing.T) {
		// Given
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-cm",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(cm, nil)
		resource.On("Identity").Return("v1/ConfigMap/missing-cm")

		// When
		results, err := readResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.Error(t, err)
		assert.Nil(t, results)
		assert.True(t, apierrors.IsNotFound(errors.Unwrap(err)))

		resource.AssertExpectations(t)
	})

	t.Run("should record the fetched object when the resource implements ObservationRecorder", func(t *testing.T) {
		// Given a cluster Secret with populated data and a read-only resource
		// constructed with an empty base; the framework must hand the fetched
		// object to RecordObservation so that subsequent data extraction sees
		// the cluster state, not the inert base.
		clusterSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "observed-secret",
				Namespace: namespace,
			},
			Data: map[string][]byte{"token": []byte("from-cluster")},
		}
		require.NoError(t, fakeClient.Create(ctx, clusterSecret))

		baseObject := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "observed-secret",
				Namespace: namespace,
			},
		}

		resource := &MockObservationRecorderResource{}
		resource.On("Object").Return(baseObject, nil)
		resource.On("RecordObservation", mock.MatchedBy(func(obj client.Object) bool {
			secret, ok := obj.(*corev1.Secret)
			if !ok {
				return false
			}
			return string(secret.Data["token"]) == "from-cluster"
		})).Return(nil)

		// When
		_, err := readResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.NoError(t, err)
		resource.AssertExpectations(t)
	})

	t.Run("should propagate errors from RecordObservation", func(t *testing.T) {
		clusterSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "record-fail-secret",
				Namespace: namespace,
			},
		}
		require.NoError(t, fakeClient.Create(ctx, clusterSecret))

		resource := &MockObservationRecorderResource{}
		resource.On("Object").Return(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "record-fail-secret",
				Namespace: namespace,
			},
		}, nil)
		resource.On("Identity").Return("v1/Secret/record-fail-secret")
		resource.On("RecordObservation", mock.Anything).Return(errors.New("record failed"))

		_, err := readResources(ctx, reconcileContext, []Resource{resource})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "record failed")
		assert.Contains(t, err.Error(), "v1/Secret/record-fail-secret")
	})

	t.Run("should return error and update status if resource.ConvergingStatus() fails", func(t *testing.T) {
		// Given
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm-alive-fail",
				Namespace: namespace,
			},
		}
		err := fakeClient.Create(ctx, cm)
		require.NoError(t, err)

		resource := &MockAliveResource{}
		resource.On("Object").Return(cm, nil)
		resource.On("Identity").Return("v1/ConfigMap/test-cm-alive-fail")
		resource.On("ConvergingStatus", concepts.ConvergingOperationNone).Return(concepts.AliveStatusWithReason{}, errors.New("failed status"))

		// When
		results, err := readResources(ctx, reconcileContext, []Resource{resource})

		// Then
		require.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "failed status")

		resource.AssertExpectations(t)
	})
}
